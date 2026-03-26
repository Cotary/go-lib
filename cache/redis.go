package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

type RedisConfig struct {
	Client     redis.UniversalClient
	Prefix     string
	DefaultTTL time.Duration
	Codec      Codec
}

type redisCache[T any] struct {
	client     redis.UniversalClient
	prefix     string
	defaultTTL time.Duration
	codec      Codec
	sf         singleflight.Group
}

func NewRedis[T any](cfg RedisConfig) (Cache[T], error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("cache: Redis Client must not be nil")
	}
	if cfg.DefaultTTL <= 0 {
		return nil, fmt.Errorf("cache: Redis DefaultTTL must be positive")
	}

	codec := cfg.Codec
	if codec == nil {
		codec = JsonCodec
	}

	return &redisCache[T]{
		client:     cfg.Client,
		prefix:     cfg.Prefix,
		defaultTTL: cfg.DefaultTTL,
		codec:      codec,
	}, nil
}

func (r *redisCache[T]) key(k string) string {
	if r.prefix == "" {
		return k
	}
	return r.prefix + ":" + k
}

func (r *redisCache[T]) ttl(opts []Option) time.Duration {
	o := applyOptions(opts)
	if o.TTL > 0 {
		return o.TTL
	}
	return r.defaultTTL
}

func (r *redisCache[T]) Get(ctx context.Context, key string) (T, error) {
	var zero T
	data, err := r.client.Get(ctx, r.key(key)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return zero, ErrNotFound
		}
		return zero, fmt.Errorf("cache: redis get: %w", err)
	}

	var v T
	if err := r.codec.Unmarshal(data, &v); err != nil {
		return zero, fmt.Errorf("cache: unmarshal: %w", err)
	}
	return v, nil
}

func (r *redisCache[T]) Set(ctx context.Context, key string, value T, opts ...Option) error {
	data, err := r.codec.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache: marshal: %w", err)
	}

	ttl := r.ttl(opts)
	if err := r.client.Set(ctx, r.key(key), data, ttl).Err(); err != nil {
		return fmt.Errorf("cache: redis set: %w", err)
	}
	return nil
}

func (r *redisCache[T]) Delete(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, r.key(key)).Err(); err != nil {
		return fmt.Errorf("cache: redis del: %w", err)
	}
	return nil
}

func (r *redisCache[T]) GetOrLoad(ctx context.Context, key string, loader LoaderFunc[T]) (T, error) {
	v, err := r.Get(ctx, key)
	if err == nil {
		return v, nil
	}
	if err != ErrNotFound {
		var zero T
		return zero, err
	}

	result, err, _ := r.sf.Do(key, func() (interface{}, error) {
		val, err := loader(ctx, key)
		if err != nil {
			return nil, err
		}
		if setErr := r.Set(ctx, key, val); setErr != nil {
			return val, fmt.Errorf("cache: loader succeeded but set failed: %w", setErr)
		}
		return val, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return result.(T), nil
}

func (r *redisCache[T]) Close() error {
	return nil
}
