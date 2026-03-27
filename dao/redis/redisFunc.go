package redis

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
)

func DbErr(err error) error {
	if errors.Is(err, redis.Nil) {
		return nil
	}
	return err
}

func (t Client) ScanKeys(ctx context.Context, prefix string) ([]string, error) {
	switch c := t.UniversalClient.(type) {
	case *redis.ClusterClient:
		return scanClusterKeys(ctx, c, prefix)
	case *redis.Client:
		return scanStandaloneKeys(ctx, c, prefix)
	default:
		return nil, fmt.Errorf("unsupported redis client type: %T", c)
	}
}

func scanStandaloneKeys(
	ctx context.Context,
	client *redis.Client,
	matchPattern string,
) ([]string, error) {
	return scanKeys(ctx, func(cursor uint64) ([]string, uint64, error) {
		return client.Scan(ctx, cursor, matchPattern, 1000).Result()
	})
}

func scanClusterKeys(
	ctx context.Context,
	cluster *redis.ClusterClient,
	matchPattern string,
) ([]string, error) {
	var keys []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	sem := make(chan struct{}, 10)
	err := cluster.ForEachMaster(ctx, func(ctx context.Context, shard *redis.Client) error {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem }()
			defer wg.Done()

			var cursor uint64
			for {
				if err := ctx.Err(); err != nil {
					select {
					case errChan <- fmt.Errorf("context cancelled: %w", err):
					default:
					}
					return
				}

				keysBatch, nextCursor, err := shard.Scan(ctx, cursor, matchPattern, 1000).Result()
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
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context cancelled: %w", err)
		}

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
