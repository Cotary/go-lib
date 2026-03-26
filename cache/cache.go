package cache

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("cache: not found")

// LoaderFunc loads a value from the origin source when cache misses.
type LoaderFunc[T any] func(ctx context.Context, key string) (T, error)

// Cache is the unified cache interface supporting memory, Redis, and two-level caches.
type Cache[T any] interface {
	Get(ctx context.Context, key string) (T, error)
	Set(ctx context.Context, key string, value T, opts ...Option) error
	Delete(ctx context.Context, key string) error
	GetOrLoad(ctx context.Context, key string, loader LoaderFunc[T]) (T, error)
	Close() error
}

type options struct {
	TTL time.Duration
}

type Option func(*options)

func WithTTL(d time.Duration) Option {
	return func(o *options) {
		o.TTL = d
	}
}

func applyOptions(opts []Option) options {
	var o options
	for _, fn := range opts {
		fn(&o)
	}
	return o
}
