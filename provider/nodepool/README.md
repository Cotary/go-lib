# nodepool - 高性能多节点请求池

协议无关的多节点请求池，支持多种调度策略和灵活的响应判定机制。**Pool 创建后即并发安全**，可在任意数量的 goroutine 中共享同一个实例。

## 特性

- **并发安全**：Pool 初始化一次，可被任意多个 goroutine 同时调用，内部通过 `sync.RWMutex` 保证线程安全
- **协议无关**：通过 `Transport` 接口抽象，支持 HTTP、gRPC、WebSocket 或任何自定义协议
- **双模式调度**：
  - **Conservative（省额度模式）**：按评分逐个尝试节点，并发安全，适合链上交易/有配额限制的 API
  - **Race（快速模式）**：并发发送到多个节点，取第一个成功结果，适合只读查询
- **三态响应判定（ResponseClassifier）**：
  - `NodeStatusSuccess`：请求成功，节点健康 +1
  - `NodeStatusFail`：节点故障（限流/超频/内部错误），触发重试，节点健康 -1
  - `NodeStatusBizError`：业务错误（参数错误/数据不存在），不重试，不影响节点健康
- **熔断器**：连续失败超阈值自动熔断，超时后半开试探恢复
- **动态节点管理**：运行时增加、删除、批量更新节点
- **自动评分排序**：后台定期按成功率、延迟、负载重新排序节点

## 快速开始

### 基础用法

```go
package main

import (
    "context"
    "fmt"
    "go-lib/provider/nodepool"
)

// 1. 实现 Transport 接口
type MyHTTPTransport struct{}

func (t *MyHTTPTransport) Execute(ctx context.Context, endpoint string, req *nodepool.Request) (*nodepool.Response, error) {
    // 你的 HTTP 请求逻辑
    return &nodepool.Response{Data: "result"}, nil
}

func main() {
    // 2. 创建节点池（只需创建一次）
    pool, err := nodepool.New(
        &MyHTTPTransport{},
        nil, // 使用默认 Classifier
        []nodepool.NodeConfig{
            {Endpoint: "https://node1.example.com", Weight: 10},
            {Endpoint: "https://node2.example.com", Weight: 5},
        },
        nodepool.WithStrategy(nodepool.StrategyConservative),
        nodepool.WithMaxRetries(3),
    )
    if err != nil {
        panic(err)
    }
    defer pool.Close()

    // 3. 发送请求
    resp, err := pool.Do(context.Background(), &nodepool.Request{
        Data: map[string]any{"method": "getBalance", "address": "0x..."},
    })
    if err != nil {
        panic(err)
    }
    fmt.Println(resp.Data)
}
```

### 并发安全 — 多 goroutine 共享同一个 Pool

Pool 是完全并发安全的，通常在程序初始化时创建一次，然后在所有 goroutine 中共享使用：

```go
package main

import (
    "context"
    "fmt"
    "sync"
    "go-lib/provider/nodepool"
)

// 全局共享 pool，初始化一次，到处使用
var rpcPool *nodepool.Pool

func init() {
    var err error
    rpcPool, err = nodepool.New(
        &MyRPCTransport{},
        &MyClassifier{},
        []nodepool.NodeConfig{
            {Endpoint: "https://rpc1.example.com", Weight: 10},
            {Endpoint: "https://rpc2.example.com", Weight: 5},
            {Endpoint: "https://rpc3.example.com", Weight: 3},
        },
        nodepool.WithStrategy(nodepool.StrategyConservative),
        nodepool.WithMaxRetries(3),
        nodepool.WithFailureThreshold(5),
    )
    if err != nil {
        panic(err)
    }
}

func main() {
    defer rpcPool.Close()

    var wg sync.WaitGroup

    // 模拟 1000 个并发请求，全部共享同一个 pool
    for i := 0; i < 1000; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()

            resp, err := rpcPool.Do(context.Background(), &nodepool.Request{
                Data: map[string]any{
                    "jsonrpc": "2.0",
                    "method":  "eth_getBalance",
                    "params":  []any{fmt.Sprintf("0xaddr_%d", id), "latest"},
                    "id":      id,
                },
            })
            if err != nil {
                fmt.Printf("request %d failed: %v\n", id, err)
                return
            }
            fmt.Printf("request %d ok: %v\n", id, resp.Data)
        }(i)
    }

    wg.Wait()
    fmt.Println("all done")

    // 查看节点运行状况
    for _, s := range rpcPool.NodeStats() {
        fmt.Printf("node=%s total=%d success=%d fail=%d avg_latency=%v circuit_open=%v\n",
            s.Endpoint, s.TotalRequests, s.SuccessCount, s.FailCount, s.AvgLatency, s.CircuitOpen)
    }
}
```

也可以作为结构体字段注入，用于服务/handler 等场景：

```go
type BlockchainService struct {
    pool *nodepool.Pool
}

func NewBlockchainService(pool *nodepool.Pool) *BlockchainService {
    return &BlockchainService{pool: pool}
}

// 多个 handler 并发调用，共享同一个 pool，完全安全
func (s *BlockchainService) GetBalance(ctx context.Context, address string) (string, error) {
    resp, err := s.pool.Do(ctx, &nodepool.Request{
        Data: map[string]any{"method": "eth_getBalance", "params": []any{address, "latest"}},
    })
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("%v", resp.Data), nil
}
```

### 自定义 ResponseClassifier（处理限流等场景）

`Classify` 方法的 `endpoint` 参数标识了当前响应来自哪个节点，可以用于区分不同节点的错误格式：

```go
type RPCClassifier struct{}

func (c *RPCClassifier) Classify(ctx context.Context, endpoint string, resp *nodepool.Response, err error) nodepool.NodeStatus {
    if err != nil {
        return nodepool.NodeStatusFail
    }
    
    body := resp.Data.(map[string]any)
    
    // 限流/超频 -> 节点失败，重试其他节点
    if errMsg, ok := body["error"].(string); ok {
        if strings.Contains(errMsg, "rate limit") || strings.Contains(errMsg, "exceeded") {
            return nodepool.NodeStatusFail
        }
        // 其他业务错误 -> 不重试
        return nodepool.NodeStatusBizError
    }
    
    return nodepool.NodeStatusSuccess
}
```

### Per-Node Classifier（每节点独立分类器）

不同供应商的错误格式可能完全不同（例如 Alchemy 返回 JSON-RPC error code，而 Infura 返回纯文本错误）。可以通过 `NodeConfig.Classifier` 为每个节点指定独立的分类器，未设置则回退到池级分类器：

```go
pool, _ := nodepool.New(
    transport,
    &DefaultRPCClassifier{},  // 池级默认分类器
    []nodepool.NodeConfig{
        {
            Endpoint: "https://eth-mainnet.alchemyapi.io",
            Weight:   10,
            Classifier: &AlchemyClassifier{},  // Alchemy 专用分类器
        },
        {
            Endpoint: "https://mainnet.infura.io",
            Weight:   5,
            Classifier: &InfuraClassifier{},   // Infura 专用分类器
        },
        {
            Endpoint: "https://my-private-node.com",
            Weight:   3,
            // Classifier 为 nil，自动使用池级的 DefaultRPCClassifier
        },
    },
)

// AlchemyClassifier 处理 Alchemy 的 JSON-RPC 错误格式
type AlchemyClassifier struct{}

func (c *AlchemyClassifier) Classify(_ context.Context, _ string, resp *nodepool.Response, err error) nodepool.NodeStatus {
    if err != nil {
        return nodepool.NodeStatusFail
    }
    body := resp.Data.(map[string]any)
    if errObj, ok := body["error"].(map[string]any); ok {
        code := int(errObj["code"].(float64))
        if code == -32005 || code == -32090 { // Alchemy 限流错误码
            return nodepool.NodeStatusFail
        }
        return nodepool.NodeStatusBizError
    }
    return nodepool.NodeStatusSuccess
}

// InfuraClassifier 处理 Infura 的纯文本错误格式
type InfuraClassifier struct{}

func (c *InfuraClassifier) Classify(_ context.Context, _ string, resp *nodepool.Response, err error) nodepool.NodeStatus {
    if err != nil {
        return nodepool.NodeStatusFail
    }
    body := resp.Data.(map[string]any)
    if errStr, ok := body["error"].(string); ok {
        if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "daily request") {
            return nodepool.NodeStatusFail
        }
        return nodepool.NodeStatusBizError
    }
    return nodepool.NodeStatusSuccess
}
```

优先级链：`NodeConfig.Classifier` > 池级 `classifier` 参数 > `DefaultClassifier`。

### 快速模式（Race）

```go
pool, _ := nodepool.New(
    transport,
    classifier,
    nodes,
    nodepool.WithStrategy(nodepool.StrategyRace),
    nodepool.WithRaceConcurrency(3), // 最多并发 3 个节点，0 = 全部
)

// 并发查询，最快的节点返回结果
resp, err := pool.Do(ctx, &nodepool.Request{Data: query})
```

### 动态节点管理

```go
// 添加节点
pool.AddNode(nodepool.NodeConfig{Endpoint: "https://node3.example.com"})

// 删除节点
pool.RemoveNode("https://node1.example.com")

// 批量同步（新增 c，保留 b，删除 a）
pool.UpdateNodes([]nodepool.NodeConfig{
    {Endpoint: "https://b.example.com"},
    {Endpoint: "https://c.example.com"},
})
```

### 批量请求

```go
reqs := []*nodepool.Request{
    {Data: req1},
    {Data: req2},
    {Data: req3},
}
responses, errs := pool.DoMulti(ctx, reqs)
```

### Per-Node 速率限制

不同节点可以设置独立的速率限制（QPS、QPM、最大并发数），超出限制的请求会自动跳到下一个可用节点：

```go
pool, _ := nodepool.New(
    transport,
    classifier,
    []nodepool.NodeConfig{
        {
            Endpoint: "https://alchemy.io",
            Weight:   10,
            RateLimit: &nodepool.RateLimitConfig{
                MaxConcurrent: 100,    // 最多同时 100 个并发请求
                PerSecond:     30,     // 每秒最多 30 个请求
                PerMinute:     1000,   // 每分钟最多 1000 个请求
                Burst:         30,     // 令牌桶突发容量
            },
        },
        {
            Endpoint: "https://infura.io",
            Weight:   5,
            RateLimit: &nodepool.RateLimitConfig{
                PerSecond: 10,
                PerMinute: 300,
            },
        },
        {
            Endpoint: "https://my-node.com",
            Weight:   3,
            // 没有 RateLimit → 不限流
        },
    },
    nodepool.WithWaitOnThrottle(true), // 全部节点限流时等待，而非返回错误
)
```

限流行为：
- **Conservative 模式**：逐个节点尝试，被限流的节点直接跳过（不扣分），试下一个
- **Race 模式**：只将请求发给当前未被限流的节点
- **全部限流**：`WithWaitOnThrottle(true)` 时阻塞等待最佳节点恢复，`false` 时返回 `ErrAllThrottled`

### DoUntilComplete — 批量请求全部完成模式

`DoMulti` 发完就返回（包含失败），`DoUntilComplete` 会自动重试失败的请求直到全部成功或 context 取消：

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

reqs := []*nodepool.Request{
    {Data: req1},
    {Data: req2},
    {Data: req3},
}
responses, errs := pool.DoUntilComplete(ctx, reqs)

// 行为：
// - 成功的请求：直接存入结果
// - NodeFail（节点故障）：自动退避重试，直到成功或 ctx 超时
// - BizError（业务错误）：不重试，直接返回错误和响应
// - ctx 取消：剩余未完成的请求标记为 ctx.Err()
```

对比 `DoMulti` 和 `DoUntilComplete`：

| 特性 | DoMulti | DoUntilComplete |
|------|---------|-----------------|
| 失败处理 | 返回错误，调用方自行重试 | 自动退避重试 |
| BizError | 返回 | 返回（不重试） |
| 阻塞行为 | 所有请求执行完就返回 | 等到全部成功或 ctx 取消 |
| 退避策略 | 无 | round × 500ms，上限 5s |
| 适用场景 | 快速批量，允许部分失败 | 必须全部完成（如批量上链） |

## 配置选项

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `WithStrategy` | 调度策略（Conservative / Race） | Conservative |
| `WithMaxRetries` | Conservative 模式最大重试轮次 | 3 |
| `WithRaceConcurrency` | Race 模式并发节点数（0=全部） | 0 |
| `WithFailureThreshold` | 连续失败多少次触发熔断 | 5 |
| `WithSuccessThreshold` | 半开状态连续成功多少次恢复 | 3 |
| `WithCircuitOpenTime` | 熔断持续时间 | 10s |
| `WithSortPeriod` | 节点排序刷新间隔 | 3s |
| `WithEWMAAlpha` | 延迟 EWMA 平滑系数 | 0.3 |
| `WithWaitOnThrottle` | 全部节点限流时是否阻塞等待 | false |

## 技术原理

### 并发模型：Per-Request Goroutine

nodepool 采用 **per-request goroutine** 模型，每个 `pool.Do()` 调用在调用方自己的 goroutine 中独立执行，不依赖内部 worker 池或全局队列：

```
调用方 goroutine-1:  pool.Do(req1) → 选 node1 → transport.Execute → 成功返回
调用方 goroutine-2:  pool.Do(req2) → 选 node1 → transport.Execute → 失败 → 选 node2 → 成功
调用方 goroutine-3:  pool.Do(req3) → 选 node1 → transport.Execute → 成功返回
...
调用方 goroutine-N:  pool.Do(reqN) → ...
```

每个请求**自治决策**：独立选节点、独立重试、独立判定响应。多个请求之间通过共享的节点评分和健康状态来间接协调。

### 锁机制

使用两层锁，互不干扰：

**Pool 级别 — `sync.RWMutex`**

- 读操作（`Do`/`NodeStats`/`NodeCount`）：使用 `RLock`，多个读可同时并发，互不阻塞
- 写操作（`AddNode`/`RemoveNode`/`UpdateNodes`/后台排序）：使用 `Lock`，独占访问

`Do()` 只在获取节点快照时短暂持有读锁，立即释放后在无锁状态下执行网络请求：

```go
func (p *Pool) Do(ctx context.Context, req *Request) (*Response, error) {
    p.mu.RLock()
    sorted := make([]*Node, len(p.nodes))
    copy(sorted, p.nodes)     // 复制快照
    p.mu.RUnlock()             // 立即释放，后续执行无锁
    // ...执行网络请求，不持有任何锁...
}
```

**Node 级别 — 每个节点独立的 `sync.RWMutex`**

- 每个 Node 有自己的锁，node1 的锁不影响 node2
- `RecordSuccess`/`RecordFailure` 使用写锁，但只做几个整数运算，耗时极短（纳秒级）
- `IsAvailable`/`Score` 使用读锁

### 请求分发流程（以 1000 并发请求为例）

假设 3 个节点，评分排序为 node1(9分) > node2(5分) > node3(2分)：

```
t=0ms     1000 个 goroutine 同时调用 pool.Do()
          全部拿到读锁获取节点快照 [node1, node2, node3]（RLock 可并发，不阻塞）
          全部选择评分最高的 node1，发起 1000 个并发请求

t=50ms    800 个请求成功返回
          → 调用 node1.RecordSuccess()（写锁，纳秒级，不阻塞网络请求）
          → 直接返回给调用方
          
          200 个请求被 node1 限流，Classifier 返回 NodeStatusFail
          → 调用 node1.RecordFailure()
          → 自动选择 node2 重试

t=100ms   200 个请求到 node2，190 个成功，10 个继续重试 node3

t=150ms   10 个请求到 node3，全部成功

t=3000ms  后台排序定时器触发，拿写锁重新排序
          node1 因大量失败导致评分下降，新排序: node2 > node3 > node1
          
t=3001ms  后续新请求拿到新快照，优先选 node2，node1 得到恢复时间
```

### Race 模式的并发控制

Race 模式下每个 `Do()` 调用会启动 N 个子 goroutine（N = 候选节点数），通过 `context.WithCancel` 实现取消：

```
pool.Do(req) 
  ├─ go transport.Execute(node1, req)  →  最先返回成功  →  cancel() 取消其他
  ├─ go transport.Execute(node2, req)  →  收到 ctx.Done，停止
  └─ go transport.Execute(node3, req)  →  收到 ctx.Done，停止
```

第一个成功的结果通过 channel 返回给调用方，同时 `cancel()` 通知所有其他 goroutine 停止。Transport 实现应当尊重 `ctx.Done()` 以及时释放资源。

### 熔断器状态机

每个节点内置三态熔断器：

```
                  连续失败 >= FailureThreshold
  ┌────────┐  ──────────────────────────────→  ┌──────────┐
  │ Closed │                                    │   Open   │
  │(正常)  │  ←─────────────────────────────── │ (熔断中) │
  └────────┘    半开状态连续成功 >= threshold     └──────────┘
       ↑                                            │
       │         超过 CircuitOpenTime                │
       │        ┌────────────┐                      │
       └────────│ Half-Open  │ ←────────────────────┘
                │ (试探中)    │
                └────────────┘
```

- **Closed**：正常接受请求
- **Open**：拒绝请求（`IsAvailable()` 返回 false），请求自动跳过该节点
- **Half-Open**：超时后允许少量请求试探，连续成功则恢复为 Closed

### 节点评分公式

```
score = weight × successRate + latencyBonus - failPenalty
```

- `weight`：节点配置权重（默认 1）
- `successRate`：成功率 = successCount / totalRequests
- `latencyBonus`：延迟越低分越高，= min(1s / avgLatency, 10)
- `failPenalty`：连续失败惩罚 = consecutiveFail × 0.5
- 熔断状态下强制 score = -1（排到最后）

后台每 `SortPeriod`（默认 3 秒）按 score 降序重新排序节点列表。

## 架构总览

```
Pool (并发安全，创建一次到处使用)
 │
 ├── Strategy (Conservative / Race)
 │    │
 │    ├── Node.TryAcquire()  ← 限流检查（通过 → 执行，限流 → 跳到下一个节点）
 │    │
 │    └── DoFunc ──→ Transport.Execute() ──→ ResponseClassifier.Classify()
 │         │              │                          │
 │    Node.Release()  [网络请求]              [判定: Success/Fail/BizError]
 │                                                   │
 ├── Node[] (按 score 降序排列)  ←───── 更新健康状态 ──┘
 │    ├── Health Stats (success/fail/latency EWMA)
 │    ├── Circuit Breaker (closed → open → half-open → closed)
 │    ├── Rate Limiter (PerSecond/PerMinute token bucket + MaxConcurrent semaphore)
 │    └── Score (综合评分，后台定期重算)
 │
 ├── Background Loop (定时排序 + 健康监控)
 │
 ├── Transport (用户实现，协议无关)
 └── ResponseClassifier (用户实现，定义成功/失败/业务错误)
      └── 优先级：NodeConfig.Classifier > Pool.classifier > DefaultClassifier
```
