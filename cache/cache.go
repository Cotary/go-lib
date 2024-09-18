package cache

import (
	"context"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/eko/gocache/store/redis/v4"
	"reflect"
	"time"
)

// UseString 这些store只能用string类型
var UseString = []string{
	redis.RedisType,
}

type Cache[T any] interface {
	Get(ctx context.Context, key string) (value T, err error)
	Set(ctx context.Context, key string, value T, options ...store.Option) error
	OriginGet(key string) (value T, err error)
}

// BaseCache T 是实际类型，U 是缓存类型
type BaseCache[T, U any] struct {
	ctx    context.Context
	config Config[T]
	cache  *cache.Cache[U]
	store  store.StoreInterface
}

type Config[T any] struct {
	Prefix     string
	Expire     time.Duration
	OriginFunc func(ctx context.Context, key string) (value T, err error)
}

func NewStore[T any, U any](ctx context.Context, config Config[T], store store.StoreInterface) *BaseCache[T, U] {
	return &BaseCache[T, U]{
		ctx:    ctx,
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
	if reflect.TypeOf(val) != reflect.TypeOf(value) {
		err = utils.AnyToAny(val, &value)
		if err != nil {
			return value, e.Err(err)
		}
		return value, nil
	}
	value = reflect.ValueOf(val).Convert(reflect.TypeOf(value)).Interface().(T)
	return value, nil
}

func (c *BaseCache[T, U]) Set(ctx context.Context, key string, value T, options ...store.Option) error {
	var cacheValue U
	key = c.GetKey(key)
	if c.config.Expire >= 0 {
		options = append(options, store.WithExpiration(c.config.Expire))
	}

	if reflect.TypeOf(value) != reflect.TypeOf(cacheValue) {
		err := utils.AnyToAny(value, &cacheValue)
		if err != nil {
			return e.Err(err)
		}
	} else {
		cacheValue = reflect.ValueOf(value).Convert(reflect.TypeOf(cacheValue)).Interface().(U)
	}
	err := c.cache.Set(ctx, key, cacheValue, options...)
	return e.Err(err)
}

func (c *BaseCache[T, U]) OriginGet(key string) (value T, err error) {
	value, err = c.Get(c.ctx, key)
	if err != nil {
		if err.Error() == store.NOT_FOUND_ERR {
			v, err := c.config.OriginFunc(c.ctx, key)
			if err != nil {
				return *new(T), e.Err(err)
			}
			err = c.Set(c.ctx, key, v)
			if err != nil {
				e.SendMessage(c.ctx, err)
			}
			return v, nil
		}
		return *new(T), e.Err(err)
	}
	return value, nil
}
