# dlock - 统一分布式锁

提供 **Memory / Redis / etcd** 三种后端的统一互斥锁抽象，通过同一套 `Provider` + `Mutex` 接口使用，可按需切换后端而无需修改业务代码。

## 核心接口

```go
// 锁工厂 —— 每种后端实现一次
type Provider interface {
    NewMutex(key string) Mutex
}

// 互斥锁 —— 所有后端统一行为
type Mutex interface {
    Lock(ctx context.Context) error      // 阻塞获取，支持 ctx 取消/超时
    TryLock(ctx context.Context) error   // 非阻塞尝试，失败返回 ErrLockFailed
    Unlock(ctx context.Context) error    // 释放锁
}
```

## 快速上手

### Memory（单进程 per-key 锁）

无需外部依赖，进程内即可使用。

```go
import "go-lib/dlock"

p := dlock.NewMemoryProvider()
m := p.NewMutex("order:create:123")

if err := m.Lock(ctx); err != nil {
    return err
}
defer m.Unlock(ctx)
// ... 临界区 ...
```

### Redis（基于 redsync）

需要一个 `go-redis/v9` 客户端。

```go
import (
    "go-lib/dlock"
    "github.com/redis/go-redis/v9"
    goredisv9 "github.com/go-redsync/redsync/v4/redis/goredis/v9"
)

rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
pool := goredisv9.NewPool(rdb)

p := dlock.NewRedisProvider(pool,
    dlock.WithRedisExpiry(10*time.Second),   // 锁过期时间，默认 8s
    dlock.WithRedisTries(32),                // 最大重试次数，默认 32
    dlock.WithRedisRetryDelay(500*time.Millisecond), // 重试间隔，默认 500ms
    dlock.WithRedisKeyPrefix("myapp:lock:"),         // key 前缀，默认 "dlock:"
)

m := p.NewMutex("order:create:123")

if err := m.Lock(ctx); err != nil {
    return err
}
defer m.Unlock(ctx)
```

### etcd（基于 etcd concurrency）

需要一个 `etcd/client/v3` 客户端。

```go
import (
    "go-lib/dlock"
    clientv3 "go.etcd.io/etcd/client/v3"
)

cli, err := clientv3.New(clientv3.Config{
    Endpoints: []string{"localhost:2379"},
})
if err != nil {
    return err
}
defer cli.Close()

p := dlock.NewEtcdProvider(cli,
    dlock.WithEtcdTTL(10),                   // Session TTL（秒），默认 10
    dlock.WithEtcdKeyPrefix("/myapp/lock/"), // key 前缀，默认 "/dlock/"
)

m := p.NewMutex("order:create:123")

if err := m.Lock(ctx); err != nil {
    return err
}
defer m.Unlock(ctx)
```

### TryLock 非阻塞用法

```go
m := p.NewMutex("dedup:task:42")

err := m.TryLock(ctx)
if errors.Is(err, dlock.ErrLockFailed) {
    // 别人已持有锁，跳过本次执行
    return nil
}
if err != nil {
    return err // 其它错误（网络、超时等）
}
defer m.Unlock(ctx)
// ... 执行任务 ...
```

## 协程安全性

| 操作 | 是否协程安全 | 说明 |
|------|:----------:|------|
| `Provider.NewMutex()` | **是** | 三种后端的 NewMutex 均可安全地从多个协程并发调用 |
| 同一 Mutex 实例 | **否** | 单个 Mutex 实例应遵循 `Lock → 操作 → Unlock` 的单一所有者模式，不要在多个协程间共享同一个 Mutex 实例并发调用 Lock/Unlock |
| 同 key 不同 Mutex 实例 | **是** | 对同一个 key 通过 `NewMutex` 创建多个实例，它们之间能正确互斥。这是推荐的并发使用方式 |

**推荐模式：** 每个需要获取锁的协程独立调用 `p.NewMutex(key)` 获取自己的 Mutex 实例。

```go
// 正确 —— 每个协程拥有自己的 Mutex 实例
go func() {
    m := p.NewMutex("shared-resource")
    _ = m.Lock(ctx)
    defer m.Unlock(ctx)
    // ...
}()

// 错误 —— 多个协程共享同一个 Mutex 实例并发 Lock/Unlock
m := p.NewMutex("shared-resource")
go func() { m.Lock(ctx); /* ... */ m.Unlock(ctx) }()
go func() { m.Lock(ctx); /* ... */ m.Unlock(ctx) }() // 未定义行为
```

### 各后端协程安全细节

- **Memory**：底层使用 `chan struct{}` (cap=1) 作为信号量，channel 本身是协程安全的。`MemoryProvider` 内部用 `sync.Mutex` 保护 map 访问。
- **Redis**：底层 `redsync.Mutex` 每个实例维护独立的 value（用于验证解锁身份），因此每个 `redisMutex` 实例独立工作。
- **etcd**：每次 `Lock` 创建独立的 `Session` + `concurrency.Mutex`，`Unlock` 后释放。实例间完全隔离。

## 选型指南

| 维度 | Memory | Redis | etcd |
|------|--------|-------|------|
| **适用范围** | 单进程 | 跨进程/跨机器 | 跨进程/跨机器 |
| **外部依赖** | 无 | Redis | etcd 集群 |
| **一致性模型** | 强一致（进程内） | AP（单节点）/ 最终一致 | CP（强一致） |
| **性能** | 纳秒级 | 毫秒级（网络 RTT） | 毫秒级（Raft 共识） |
| **崩溃自动释放** | 进程退出即释放 | 靠 key 过期（TTL） | 靠 Lease 过期（TTL） |
| **典型场景** | 本地去重、单实例定时任务 | 防止重复计算、接口幂等 | 分布式选主、强一致资源调度 |

**简单决策：**

1. 单实例部署 / 本地测试 → **Memory**
2. 多实例部署，偶尔重复执行可接受（效率型） → **Redis**
3. 多实例部署，绝对不能重复执行（正确性型） → **etcd**

## 配置参数参考

### Redis

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `WithRedisExpiry` | 8s | 锁的 Redis key 过期时间。应大于业务临界区最长执行时间 |
| `WithRedisTries` | 32 | `Lock` 最大重试次数。TryLock 始终只尝试一次 |
| `WithRedisRetryDelay` | 500ms | 重试间隔 |
| `WithRedisKeyPrefix` | `dlock:` | Redis key 前缀，用于区分不同应用/环境 |

### etcd

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `WithEtcdTTL` | 10（秒） | Session 租约 TTL。持锁进程崩溃后，锁最长在此时间后自动释放 |
| `WithEtcdKeyPrefix` | `/dlock/` | etcd key 前缀 |

## 注意事项

1. **锁过期 vs 业务耗时**：Redis/etcd 的锁都有 TTL。如果业务操作时间可能超过 TTL，锁会被自动释放，其他进程可能获取到锁。请确保 TTL 大于最长业务执行时间，或在业务侧做好幂等保护。

2. **Redis 单节点限制**：`redsync` 在单个 Redis 节点下退化为 `SET NX PX`，不提供 Redlock 的容错能力。如果 Redis 发生故障转移（主从切换），可能出现短暂的锁失效。对强一致场景请使用 etcd。

3. **etcd Session 生命周期**：本实现在每次 `Lock` 时创建新 Session，`Unlock` 时释放。这避免了 Session 复用的生命周期管理问题，但每次加锁有额外的 Session 创建开销（通常 < 10ms）。

4. **Memory 后端不跨进程**：`MemoryProvider` 的锁仅在当前进程内有效，多实例部署时各进程的内存锁完全独立，无法实现跨进程互斥。

5. **与 `common/utils/singleRun.go` 的关系**：`SingleRun` 是进程内的 per-key 执行管理器，支持等待超时、嵌套调用等高级语义。`dlock.MemoryProvider` 提供更纯粹的锁原语。如果只需要简单的 Lock/Unlock，用 `dlock`；如果需要等待队列、运行状态查询、嵌套调用支持，继续使用 `SingleRun`。
