package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/maypok86/otter/v2"
)

type MemoryConfig struct {
	MaxSize    int
	DefaultTTL time.Duration
}

type memoryCache[T any] struct {
	inner      *otter.Cache[string, T]
	defaultTTL time.Duration
}

func NewMemory[T any](cfg MemoryConfig) (Cache[T], error) {
	if cfg.MaxSize <= 0 {
		return nil, fmt.Errorf("cache: MaxSize must be positive")
	}

	opts := &otter.Options[string, T]{
		MaximumSize: cfg.MaxSize,
	}
	if cfg.DefaultTTL > 0 {
		opts.ExpiryCalculator = otter.ExpiryWriting[string, T](cfg.DefaultTTL)
	}

	c, err := otter.New(opts)
	if err != nil {
		return nil, fmt.Errorf("cache: failed to create otter cache: %w", err)
	}

	return &memoryCache[T]{
		inner:      c,
		defaultTTL: cfg.DefaultTTL,
	}, nil
}

func (m *memoryCache[T]) Get(_ context.Context, key string) (T, error) {
	v, ok := m.inner.GetIfPresent(key)
	if !ok {
		var zero T
		return zero, ErrNotFound
	}
	return v, nil
}

func (m *memoryCache[T]) Set(_ context.Context, key string, value T, opts ...Option) error {
	m.inner.Set(key, value)

	o := applyOptions(opts)
	if o.TTL > 0 {
		m.inner.SetExpiresAfter(key, o.TTL)
	}
	return nil
}

func (m *memoryCache[T]) Delete(_ context.Context, key string) error {
	m.inner.Invalidate(key)
	return nil
}

func (m *memoryCache[T]) GetOrLoad(ctx context.Context, key string, loader LoaderFunc[T]) (T, error) {
	return m.inner.Get(ctx, key, otter.LoaderFunc[string, T](func(ctx context.Context, key string) (T, error) {
		return loader(ctx, key)
	}))
}

func (m *memoryCache[T]) Close() error {
	m.inner.StopAllGoroutines()
	return nil
}
