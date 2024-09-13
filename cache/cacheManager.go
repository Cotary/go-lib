package cache

import (
	"context"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/pkg/errors"
	"sync"
)

var cacheMap sync.Map

func StoreInstance[T any](ctx context.Context, config Config[T], store store.StoreInterface) *BaseCache[T] {
	instance, err := GetCacheInstance[T](config.Prefix)
	if err == nil {
		return instance
	}
	storeInstance := NewStore[T](ctx, config, store)
	cacheMap.Store(config.Prefix, storeInstance)
	return storeInstance
}

func GetCacheInstance[T any](key string) (*BaseCache[T], error) {
	if cache, ok := cacheMap.Load(key); ok {
		if typedCache, ok := cache.(*BaseCache[T]); ok {
			return typedCache, nil
		} else {
			return nil, errors.New("cache instance type error")
		}
	} else {
		return nil, errors.New("cache instance not found")
	}

}
