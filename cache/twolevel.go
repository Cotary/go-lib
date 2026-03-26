package cache

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sync/singleflight"
)

type TwoLevelConfig struct {
	Local  MemoryConfig
	Remote RedisConfig
}

type twoLevelCache[T any] struct {
	local  Cache[T]
	remote Cache[T]
	sf     singleflight.Group
}

func NewTwoLevel[T any](cfg TwoLevelConfig) (Cache[T], error) {
	local, err := NewMemory[T](cfg.Local)
	if err != nil {
		return nil, fmt.Errorf("cache: create local cache: %w", err)
	}

	remote, err := NewRedis[T](cfg.Remote)
	if err != nil {
		local.Close()
		return nil, fmt.Errorf("cache: create remote cache: %w", err)
	}

	return &twoLevelCache[T]{
		local:  local,
		remote: remote,
	}, nil
}

func (t *twoLevelCache[T]) Get(ctx context.Context, key string) (T, error) {
	v, err := t.local.Get(ctx, key)
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return v, err
	}

	v, err = t.remote.Get(ctx, key)
	if err == nil {
		_ = t.local.Set(ctx, key, v)
		return v, nil
	}
	return v, err
}

func (t *twoLevelCache[T]) Set(ctx context.Context, key string, value T, opts ...Option) error {
	if err := t.remote.Set(ctx, key, value, opts...); err != nil {
		return err
	}
	_ = t.local.Set(ctx, key, value, opts...)
	return nil
}

func (t *twoLevelCache[T]) Delete(ctx context.Context, key string) error {
	remoteErr := t.remote.Delete(ctx, key)
	_ = t.local.Delete(ctx, key)
	return remoteErr
}

func (t *twoLevelCache[T]) GetOrLoad(ctx context.Context, key string, loader LoaderFunc[T]) (T, error) {
	v, err := t.local.Get(ctx, key)
	if err == nil {
		return v, nil
	}

	result, err, _ := t.sf.Do(key, func() (interface{}, error) {
		// Check remote first (inside singleflight to avoid thundering herd on Redis)
		val, err := t.remote.Get(ctx, key)
		if err == nil {
			return val, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}

		val, err = loader(ctx, key)
		if err != nil {
			return nil, err
		}

		if setErr := t.remote.Set(ctx, key, val); setErr != nil {
			return val, fmt.Errorf("cache: loader succeeded but remote set failed: %w", setErr)
		}
		return val, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}

	v = result.(T)
	_ = t.local.Set(ctx, key, v)
	return v, nil
}

func (t *twoLevelCache[T]) Close() error {
	return t.local.Close()
}
