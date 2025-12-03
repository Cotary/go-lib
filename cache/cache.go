package cache

import (
	"context"
	"time"

	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/eko/gocache/store/redis/v4"
)

// UseString 这些store只能用string类型
var UseString = []string{
	redis.RedisType,
}

type Cache[T any] interface {
	Get(ctx context.Context, key string) (value T, err error)
	Set(ctx context.Context, key string, value T, options ...store.Option) error
	OriginGet(ctx context.Context, key string) (value T, err error)
}

// BaseCache T 是实际类型，U 是缓存类型
type BaseCache[T, U any] struct {
	config Config[T]
	cache  *cache.Cache[U]
	store  store.StoreInterface
}

type Config[T any] struct {
	Prefix     string
	Expire     time.Duration
	OriginFunc func(ctx context.Context, key string) (value T, err error)
}

func NewStore[T any, U any](config Config[T], store store.StoreInterface) *BaseCache[T, U] {
	return &BaseCache[T, U]{
		config: config,
		cache:  cache.New[U](store),
		store:  store,
	}
}

func (c *BaseCache[T, U]) GetKey(key string) string {
	key = c.config.Prefix + "_" + key
	if storeKey, ok := c.store.(interface {
		Key(key string) string
	}); ok {
		return storeKey.Key(key)
	}
	return key
}

func (c *BaseCache[T, U]) Get(ctx context.Context, key string) (value T, err error) {
	val, err := c.cache.Get(ctx, c.GetKey(key))
	if err != nil {
		return value, e.Err(err)
	}
	// 快路径：类型相同直接返回
	if v, ok := any(val).(T); ok {
		return v, nil
	}
	// 类型不同，使用泛型转换 U -> T
	value, err = utils.AnyToAny[T](val)
	if err != nil {
		return value, e.Err(err)
	}
	return value, nil
}

func (c *BaseCache[T, U]) Set(ctx context.Context, key string, value T, options ...store.Option) error {
	key = c.GetKey(key)
	if c.config.Expire >= 0 {
		options = append(options, store.WithExpiration(c.config.Expire))
	}

	// 快路径：类型相同直接使用
	if v, ok := any(value).(U); ok {
		return e.Err(c.cache.Set(ctx, key, v, options...))
	}
	// 类型不同，使用泛型转换 T -> U
	cacheValue, err := utils.AnyToAny[U](value)
	if err != nil {
		return e.Err(err)
	}
	return e.Err(c.cache.Set(ctx, key, cacheValue, options...))
}

func (c *BaseCache[T, U]) OriginGet(ctx context.Context, key string) (value T, err error) {
	value, err = c.Get(ctx, key)
	if err != nil {
		if err.Error() == store.NOT_FOUND_ERR {
			v, err := c.config.OriginFunc(ctx, key)
			if err != nil {
				return *new(T), e.Err(err)
			}
			err = c.Set(ctx, key, v)
			if err != nil {
				e.SendMessage(ctx, err)
			}
			return v, nil
		}
		return *new(T), e.Err(err)
	}
	return value, nil
}
