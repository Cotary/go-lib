# cache

基于泛型的统一缓存库，支持内存缓存、Redis 缓存和两级缓存三种模式。

- **内存缓存**：基于 [otter v2](https://github.com/maypok86/otter)，W-TinyLFU 淘汰策略，极高吞吐量
- **Redis 缓存**：基于 [go-redis/v9](https://github.com/redis/go-redis)，JSON 序列化（可自定义 Codec）
- **两级缓存**：L1 内存 + L2 Redis，自动回填，singleflight 防击穿

## 统一接口

所有缓存模式共享同一个 `Cache[T]` 接口：

```go
type Cache[T any] interface {
    Get(ctx context.Context, key string) (T, error)
    Set(ctx context.Context, key string, value T, opts ...Option) error
    Delete(ctx context.Context, key string) error
    GetOrLoad(ctx context.Context, key string, loader LoaderFunc[T]) (T, error)
    Close() error
}
```

- `Get` 未命中返回 `cache.ErrNotFound`，使用 `errors.Is(err, cache.ErrNotFound)` 判断
- `GetOrLoad` 内置 singleflight，大量并发请求同一个 key 时只会调用一次 loader
- `Set` 支持 `WithTTL` 覆盖默认过期时间

## 使用方式

### 1. 内存缓存

适用于单进程热点数据、配置缓存等场景。

```go
import "go-lib/cache"

// 创建缓存，最多 10000 条，写入后 1 分钟过期
c, err := cache.NewMemory[User](cache.MemoryConfig{
    MaxSize:    10000,
    DefaultTTL: time.Minute,
})
if err != nil {
    log.Fatal(err)
}
defer c.Close()

// 写入
c.Set(ctx, "user:1001", User{Name: "alice", Age: 30})

// 读取
user, err := c.Get(ctx, "user:1001")
if errors.Is(err, cache.ErrNotFound) {
    // 缓存未命中
}

// 自动回源（内置 singleflight 防击穿）
user, err := c.GetOrLoad(ctx, "user:1001", func(ctx context.Context, key string) (User, error) {
    return db.FindUser(ctx, 1001)
})

// 删除
c.Delete(ctx, "user:1001")

// 单次覆盖 TTL
c.Set(ctx, "temp", User{}, cache.WithTTL(5*time.Second))
```

### 2. Redis 缓存

适用于分布式场景，多实例共享数据。

```go
import (
    "go-lib/cache"
    "github.com/redis/go-redis/v9"
)

rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})

c, err := cache.NewRedis[User](cache.RedisConfig{
    Client:     rdb,              // go-redis 客户端（支持单机/集群）
    Prefix:     "myapp:user",     // key 前缀，实际 key 为 myapp:user:1001
    DefaultTTL: 5 * time.Minute,  // 默认过期时间（必填，必须 > 0）
})
if err != nil {
    log.Fatal(err)
}

// 用法与内存缓存完全一致
user, err := c.GetOrLoad(ctx, "1001", fetchUserFromDB)
```

**配合 `dao/redis.Client` 使用：**

```go
import daoRedis "go-lib/dao/redis"

client, _ := daoRedis.NewRedis(&daoRedis.Config{Host: "127.0.0.1", Port: "6379"})

c, _ := cache.NewRedis[User](cache.RedisConfig{
    Client:     client.UniversalClient,  // 传入内嵌的 UniversalClient
    Prefix:     "user",
    DefaultTTL: 5 * time.Minute,
})
```

### 3. 两级缓存

适用于高 QPS 场景，L1 本地内存挡住大部分读请求，L1 未命中才查 Redis。

```go
c, err := cache.NewTwoLevel[User](cache.TwoLevelConfig{
    Local: cache.MemoryConfig{
        MaxSize:    1000,              // 本地最多缓存 1000 条
        DefaultTTL: 30 * time.Second,  // 本地过期时间设短，减少数据不一致窗口
    },
    Remote: cache.RedisConfig{
        Client:     rdb,
        Prefix:     "myapp:user",
        DefaultTTL: 5 * time.Minute,  // Redis 过期时间设长
    },
})
if err != nil {
    log.Fatal(err)
}
defer c.Close()

// 用法与其他模式完全一致
user, err := c.GetOrLoad(ctx, "1001", fetchUserFromDB)
```

**数据流：**

```
GetOrLoad("1001", loader)
  ├─ L1 命中 → 直接返回（~100ns）
  └─ L1 未命中
      └─ singleflight 合并并发请求
          ├─ L2 命中 → 回填 L1，返回（~1ms）
          └─ L2 未命中
              └─ 调用 loader 回源 → 写入 L2 + L1，返回
```

## 自定义 Codec

Redis 缓存默认使用 JSON（`json-iterator`）序列化。你可以：

```go
// 使用标准库 JSON
cache.NewRedis[User](cache.RedisConfig{
    Client:     rdb,
    Prefix:     "user",
    DefaultTTL: time.Minute,
    Codec:      cache.StdJsonCodec,
})

// 自定义 Codec（如 MsgPack）
type msgpackCodec struct{}
func (msgpackCodec) Marshal(v any) ([]byte, error)            { /* ... */ }
func (msgpackCodec) Unmarshal(data []byte, v any) error        { /* ... */ }

cache.NewRedis[User](cache.RedisConfig{
    Client:     rdb,
    Prefix:     "user",
    DefaultTTL: time.Minute,
    Codec:      msgpackCodec{},
})
```

## 文件结构

```
cache/
├── cache.go      # Cache[T] 接口、ErrNotFound、Option、LoaderFunc
├── codec.go      # Codec 接口 + JsonCodec / StdJsonCodec
├── memory.go     # NewMemory — 基于 otter v2
├── redis.go      # NewRedis  — 基于 go-redis/v9
├── twolevel.go   # NewTwoLevel — L1 内存 + L2 Redis
└── cache_test.go # 测试
```
