package cache

import (
	"context"
	"fmt"
	redis_store "github.com/eko/gocache/store/redis/v4"
	"github.com/redis/go-redis/v9"
	"testing"
	"time"

	gocache_store "github.com/eko/gocache/store/go_cache/v4"
	gocache "github.com/patrickmn/go-cache"
)

type A struct {
	Name string `json:"name"`
}

func TestRedisNotStringCache(t *testing.T) {
	ctx := context.Background()

	//创建 Redis store
	redisClient := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",
	})
	redisStore := redis_store.NewRedis(redisClient)

	//gocacheClient := gocache.New(5*time.Minute, 10*time.Minute)
	//gocacheStore := gocache_store.NewGoCache(gocacheClient)
	// 创建缓存实例
	baseCache := NewStore[string, string](Config[string]{
		Prefix: "test",
		Expire: 1 * time.Second,
		OriginFunc: func(ctx context.Context, key string) (string, error) {
			return key, nil
		},
	}, redisStore)

	// 测试 Set 和 Get 方法
	value1 := "123"
	err := baseCache.Set(ctx, "key1", value1)
	fmt.Println("test Set:", err)

	value, err := baseCache.Get(ctx, "key1")
	fmt.Println("test Get:", value, err)

	// 测试缓存过期
	time.Sleep(2 * time.Second)
	value, err = baseCache.Get(ctx, "key1")
	fmt.Println("test expire:", value, "err:", err)

	// 测试 OriginGet 方法
	value, err = baseCache.OriginGet(ctx, "key2")
	fmt.Println("test OriginGet:", value, err)

	value, err = baseCache.Get(ctx, "key2")
	fmt.Println("test OriginGet expire:", value, err)
}

func TestStringCache(t *testing.T) {
	ctx := context.Background()

	// 创建 Redis store
	//redisClient := redis.NewClient(&redis.Options{
	//	Addr: "127.0.0.1:6379",
	//})
	//redisStore := redis_store.NewRedis(redisClient)

	gocacheClient := gocache.New(5*time.Minute, 10*time.Minute)
	gocacheStore := gocache_store.NewGoCache(gocacheClient)
	// 创建缓存实例
	baseCache := StoreInstance(Config[string]{
		Prefix: "test",
		Expire: 1 * time.Second,
		OriginFunc: func(ctx context.Context, key string) (string, error) {
			return key, nil
		},
	}, gocacheStore)

	// 测试 Set 和 Get 方法
	value1 := "value1"
	err := baseCache.Set(ctx, "key1", value1)
	fmt.Println("test Set:", err)

	value, err := baseCache.Get(ctx, "key1")
	fmt.Println("test Get:", value, err)

	// 测试缓存过期
	time.Sleep(2 * time.Second)
	value, err = baseCache.Get(ctx, "key1")
	fmt.Println("test expire:", value, "err:", err)

	// 测试 OriginGet 方法
	value, err = baseCache.OriginGet(ctx, "key2")
	fmt.Println("test OriginGet:", value, err)

	value, err = baseCache.Get(ctx, "key2")
	fmt.Println("test OriginGet expire:", value, err)
}

func TestBaseCache(t *testing.T) {
	ctx := context.Background()

	// 创建 Redis store
	//redisClient := redis.NewClient(&redis.Options{
	//	Addr: "127.0.0.1:6379",
	//})
	//redisStore := redis_store.NewRedis(redisClient)

	gocacheClient := gocache.New(5*time.Minute, 10*time.Minute)
	gocacheStore := gocache_store.NewGoCache(gocacheClient)
	// 创建缓存实例
	baseCache := StoreInstance(Config[A]{
		Prefix: "test",
		Expire: 1 * time.Second,
		OriginFunc: func(ctx context.Context, key string) (A, error) {
			return A{
				Name: key,
			}, nil
		},
	}, gocacheStore)

	// 测试 Set 和 Get 方法
	key1 := A{
		Name: "value1",
	}

	err := baseCache.Set(ctx, "key1", key1)
	fmt.Println("test Set:", err)

	value, err := baseCache.Get(ctx, "key1")
	fmt.Println("test Get:", value, err)

	// 测试缓存过期
	time.Sleep(2 * time.Second)
	value, err = baseCache.Get(ctx, "key1")
	fmt.Println("test expire:", value, "err:", err)

	// 测试 OriginGet 方法
	value, err = baseCache.OriginGet(ctx, "key2")
	fmt.Println("test OriginGet:", value, err)

	value, err = baseCache.Get(ctx, "key2")
	fmt.Println("test OriginGet expire:", value, err)
}
