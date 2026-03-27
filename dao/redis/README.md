# dao/redis — Redis 客户端封装

基于 [go-redis/v9](https://github.com/redis/go-redis) 的 Redis 客户端封装，支持单机/集群模式、TLS、连接池配置、Key 前缀、命令日志 Hook 以及集群 Key 扫描。

## 快速开始

### 初始化

```go
import "github.com/Cotary/go-lib/dao/redis"

config := &redis.Config{
    Host:     "127.0.0.1",
    Port:     "6379",
    Auth:     "your_password",
    DB:       0,
    PoolSize: 20,
    Prefix:   "myapp:",
}

client, err := redis.NewRedis(config)
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

### Config 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `Host` | string | 单机模式主机地址（含端口时可省略 Port） |
| `Port` | string | 单机模式端口 |
| `Nodes` | []string | 集群模式节点列表（`host:port` 格式） |
| `Username` | string | 用户名（Redis 6+ ACL） |
| `Auth` | string | 密码 |
| `DB` | int | 数据库编号（仅单机模式） |
| `PoolSize` | int | 连接池大小，0 时默认 20 |
| `Encryption` | uint8 | 密码加密方式：1=MD5 |
| `Framework` | string | `"standalone"`（默认）或 `"cluster"` |
| `Prefix` | string | Key 前缀，`client.Key()` 时自动拼接 |
| `Tls` | bool | 是否启用 TLS 连接 |
| `MinVersion` | uint16 | TLS 最低版本，默认 TLS 1.2 |
| `ReadTimeout` | int64 | 读超时(毫秒)，默认 3000 |
| `WriteTimeout` | int64 | 写超时(毫秒)，默认 3000 |

---

## 基本用法

### 使用 go-redis 原生方法

`Client` 内嵌了 `redis.UniversalClient`，可直接调用 go-redis 的全部方法：

```go
ctx := context.Background()

// SET / GET
client.Set(ctx, client.Key("user:1"), "张三", time.Hour)
val, err := client.Get(ctx, client.Key("user:1")).Result()

// Hash
client.HSet(ctx, client.Key("user:1:info"), "name", "张三", "age", 25)
info, err := client.HGetAll(ctx, client.Key("user:1:info")).Result()

// List
client.LPush(ctx, client.Key("queue"), "task1", "task2")
task, err := client.RPop(ctx, client.Key("queue")).Result()

// Pipeline
pipe := client.Pipeline()
pipe.Set(ctx, client.Key("k1"), "v1", 0)
pipe.Set(ctx, client.Key("k2"), "v2", 0)
_, err := pipe.Exec(ctx)
```

### Key 前缀

使用 `client.Key()` 自动拼接配置的前缀，避免不同应用间的 Key 冲突：

```go
// Config.Prefix = "myapp:"
client.Key("user:1")  // → "myapp:user:1"
client.Key("order:2") // → "myapp:order:2"
```

---

## 错误处理

`DbErr` 将 `redis.Nil`（Key 不存在）视为正常情况返回 `nil`，真实错误原样透传：

```go
val, err := client.Get(ctx, "key").Result()
if err = redis.DbErr(err); err != nil {
    // 真正的 Redis 错误
    return err
}
// val 可能为空字符串（Key 不存在时）
```

---

## ScanKeys — 扫描 Key

支持单机和集群模式，集群模式下自动并发扫描所有主节点：

```go
// 扫描所有匹配 "user:*" 的 key
keys, err := client.ScanKeys(ctx, "user:*")
if err != nil {
    return err
}
fmt.Printf("found %d keys\n", len(keys))
```

**注意事项：**
- 生产环境中对大量 Key 的扫描应配合 context 超时使用
- 集群模式下并发度限制为 10，自动只扫描主节点
- ScanKeys 会检查 context 取消状态，可通过 `context.WithTimeout` 控制执行时间

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

keys, err := client.ScanKeys(ctx, "prefix:*")
```

---

## 集群模式

```go
config := &redis.Config{
    Framework: "cluster",
    Nodes:     []string{"10.0.0.1:6379", "10.0.0.2:6379", "10.0.0.3:6379"},
    Auth:      "cluster_password",
    PoolSize:  30,
}

client, err := redis.NewRedis(config)
```

如果未配置 `Nodes`，会退化为使用 `Host:Port` 作为集群入口。

---

## TLS 配置

```go
config := &redis.Config{
    Host:       "redis.example.com",
    Port:       "6380",
    Auth:       "password",
    Tls:        true,
    MinVersion: 0, // 默认 TLS 1.2，可设为 tls.VersionTLS13
}
```

---

## 日志

初始化时自动挂载 `LogHook`，记录以下事件：

| 事件 | 字段 | 说明 |
|------|------|------|
| `redis_dial` | network, addr, cost_ms | 连接建立 |
| `redis_cmd` | command, args, raw, cost_ms | 单条命令执行 |
| `redis_pipeline` | cmd_count, commands, cost_ms | Pipeline 批量执行 |

成功时记录 Info 级别；失败时记录 Error 级别，附带 `error` 字段。

---

## 配合分布式锁

项目已引入 `github.com/go-redsync/redsync/v4`，可配合使用：

```go
import "github.com/go-redsync/redsync/v4"
import "github.com/go-redsync/redsync/v4/redis/goredis/v9"

pool := goredis.NewPool(client.UniversalClient)
rs := redsync.New(pool)

mutex := rs.NewMutex("my-lock-key", redsync.WithExpiry(10*time.Second))
if err := mutex.Lock(); err != nil {
    return err
}
defer mutex.Unlock()
```
