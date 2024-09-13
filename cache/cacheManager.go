package cache

import (
	"context"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/pkg/errors"
	"sync"
)

var cacheMap sync.Map

func StoreInstance[T any](ctx context.Context, config Config[T], store store.StoreInterface) Cache[T] {
	instance, err := GetCacheInstance[T](config.Prefix)
	if err == nil {
		return instance
	}
	var storeInstance Cache[T]
	if utils.InArray(store.GetType(), UseString) {
		storeInstance = NewStore[T, string](ctx, config, store)
	} else {
		storeInstance = NewStore[T, T](ctx, config, store)

	}
	cacheMap.Store(config.Prefix, storeInstance)
	return storeInstance
}

func GetCacheInstance[T any](key string) (Cache[T], error) {
	if cache, ok := cacheMap.Load(key); ok {
		if typedCache, ok := cache.(Cache[T]); ok {
			return typedCache, nil
		} else {
			return nil, errors.New("cache instance type error")
		}
	} else {
		return nil, errors.New("cache instance not found")
	}

}
