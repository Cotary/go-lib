package cache

import (
	"context"
	"errors"
	"time"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
)

type BaseCache[T any] struct {
	ctx    context.Context
	config Config
	cache  *cache.Cache[T]
	store  store.StoreInterface
}

type Config struct {
	Prefix    string
	Expire    int64
	GetOrigin func(ctx context.Context, key string) (value any, err error)
}

func NewStore[T any](ctx context.Context, config Config, store store.StoreInterface) *BaseCache[T] {
	return &BaseCache[T]{
		ctx:    ctx,
		config: config,
		cache:  cache.New[T](store),
		store:  store,
	}
}

func (c *BaseCache[T]) GetKey(key string) string {
	key = c.config.Prefix + "_" + key
	if storeKey, ok := c.store.(interface {
		Key(key string) string
	}); ok {
		return storeKey.Key(key)
	}
	return key
}

func (c *BaseCache[T]) Get(ctx context.Context, key string) (value T, err error) {
	return c.cache.Get(ctx, c.GetKey(key))
}
func (c *BaseCache[T]) Set(ctx context.Context, key string, value T, options ...store.Option) error {
	key = c.GetKey(key)
	if c.config.Expire >= 0 {
		options = append(options, store.WithExpiration(time.Duration(c.config.Expire)*time.Second))
	}
	return c.cache.Set(ctx, key, value, options...)
}

func (c *BaseCache[T]) OriginGet(key string) (value T, err error) {
	value, err = c.Get(c.ctx, key)
	if err != nil {
		if err.Error() == store.NOT_FOUND_ERR {
			v, err := c.config.GetOrigin(c.ctx, key)
			if err != nil {
				return *new(T), err
			}
			if v == nil {
				return *new(T), store.NotFound{}
			}
			if val, ok := v.(T); ok {
				c.Set(c.ctx, key, val)
				return val, nil
			} else {
				return *new(T), errors.New("origin type error")
			}

		} else {
			return *new(T), err
		}
	}
	return value, nil
}
