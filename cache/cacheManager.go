package cache

import (
	"github.com/Cotary/go-lib/common/utils"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/pkg/errors"
	"sync"
)

var cacheMap sync.Map

func StoreInstance[T any](config Config[T], store store.StoreInterface) Cache[T] {
	if instance, err := GetCacheInstance[T](config.Prefix); err == nil {
		return instance
	}

	var storeInstance Cache[T]
	if utils.InArray(store.GetType(), UseString) {
		storeInstance = NewStore[T, string](config, store)
	} else {
		storeInstance = NewStore[T, T](config, store)
	}

	cacheMap.Store(config.Prefix, storeInstance)
	return storeInstance
}

func GetCacheInstance[T any](key string) (Cache[T], error) {
	cache, ok := cacheMap.Load(key)
	if !ok {
		return nil, errors.New("cache instance not found")
	}

	typedCache, ok := cache.(Cache[T])
	if !ok {
		return nil, errors.New("cache instance type error")
	}

	return typedCache, nil
}
