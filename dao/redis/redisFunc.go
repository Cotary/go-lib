package redis

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"strings"
	"sync"
)

func DbErr(err error) error {
	if errors.Is(err, redis.Nil) {
		return nil
	}
	return err
}

// ScanKeys 扫描单机或集群 keys
func (t Client) ScanKeys(ctx context.Context, prefix string, batchSize int64) ([]string, error) {
	if t.ClusterClient != nil {
		return scanClusterKeys(ctx, t.ClusterClient, prefix, batchSize)
	}
	return scanStandaloneKeys(ctx, t.Client, prefix, batchSize)
}

func scanStandaloneKeys(
	ctx context.Context,
	client *redis.Client,
	matchPattern string,
	batchSize int64,
) ([]string, error) {
	return scanKeys(ctx, func(cursor uint64) ([]string, uint64, error) {
		return client.Scan(ctx, cursor, matchPattern, batchSize).Result()
	})
}

func scanClusterKeys(
	ctx context.Context,
	cluster *redis.ClusterClient,
	matchPattern string,
	batchSize int64,
) ([]string, error) {
	var keys []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	sem := make(chan struct{}, 10)
	//就算只配置一个，也是可以识别到其他节点的
	err := cluster.ForEachShard(ctx, func(ctx context.Context, shard *redis.Client) error {
		// 只扫描主节点
		info, err := shard.Info(ctx, "replication").Result()
		if err != nil {
			return err
		}

		if strings.Contains(info, "role:slave") { // master/slave
			return nil
		}

		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem }()
			defer wg.Done()

			var cursor uint64
			for {
				keysBatch, nextCursor, err := shard.Scan(ctx, cursor, matchPattern, batchSize).Result()
				if err != nil {
					select {
					case errChan <- fmt.Errorf("shard %s error: %w", shard.String(), err):
					default:
					}
					return
				}

				mu.Lock()
				keys = append(keys, keysBatch...)
				mu.Unlock()

				if cursor = nextCursor; cursor == 0 {
					break
				}
			}
		}()
		return nil
	})

	// 等待所有协程完成
	wg.Wait()

	if err != nil {
		return nil, fmt.Errorf("cluster iteration failed: %w", err)
	}

	select {
	case err := <-errChan:
		return nil, err
	default:
		return keys, nil
	}
}

func scanKeys(
	ctx context.Context,
	scanFunc func(cursor uint64) ([]string, uint64, error),
) ([]string, error) {
	var keys []string
	var cursor uint64

	for {
		keysBatch, nextCursor, err := scanFunc(cursor)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}

		keys = append(keys, keysBatch...)
		cursor = nextCursor

		if cursor == 0 {
			break
		}
	}

	return keys, nil
}
