// Package nodepool - 完整使用示例
// 演示所有核心功能：Transport实现、Classifier定制、Conservative/Race模式、
// 动态节点管理、并发安全使用、批量请求、节点统计
package nodepool_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Cotary/go-lib/provider/nodepool"
)

// ============================================================
// 第一步：实现 Transport 接口
// Transport 负责"怎么发请求"，和具体协议相关，由使用方实现
// ============================================================

// MockTransport 模拟一个真实的 HTTP/RPC 请求，这里用 sleep 代替网络延迟
type MockTransport struct{}

func (t *MockTransport) Execute(ctx context.Context, endpoint string, req *nodepool.Request) (*nodepool.Response, error) {
	// 从请求数据中取出参数（实际场景里这里是 http.Post / grpc.Invoke 等）
	data := req.Data.(map[string]any)
	method := data["method"].(string)

	// 模拟不同节点的延迟
	delay := map[string]time.Duration{
		"https://node1.example.com": 20 * time.Millisecond,
		"https://node2.example.com": 50 * time.Millisecond,
		"https://node3.example.com": 10 * time.Millisecond,
	}
	if d, ok := delay[endpoint]; ok {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			// 上游取消了请求（超时 / 主动 cancel），立刻返回
			// Transport 必须响应 ctx.Done()，否则 Race 模式的取消无法生效
			return nil, ctx.Err()
		}
	}

	// 模拟 node1 有时候会限流（rate limit）
	if endpoint == "https://node1.example.com" && method == "getBalance" {
		// 模拟 20% 概率限流
		if time.Now().UnixMilli()%5 == 0 {
			return &nodepool.Response{
				Data: map[string]any{
					"error": "rate limit exceeded",
				},
			}, nil
		}
	}

	// 正常返回
	return &nodepool.Response{
		Data: map[string]any{
			"result":   fmt.Sprintf("data_from_%s_method_%s", endpoint, method),
			"endpoint": endpoint,
		},
	}, nil
}

// ============================================================
// 第二步：实现 ResponseClassifier 接口
// Classifier 负责"怎么判断请求结果"，区分三种情况：
//   - NodeStatusSuccess：节点正常，请求成功
//   - NodeStatusFail：节点故障（限流/超时/内部错误），重试其他节点，节点扣分
//   - NodeStatusBizError：业务错误（参数错/数据不存在），不重试，不扣节点分
// ============================================================

type RPCClassifier struct{}

func (c *RPCClassifier) Classify(_ context.Context, endpoint string, resp *nodepool.Response, err error) nodepool.NodeStatus {
	// transport 层直接报错（网络不通、超时等）→ 节点故障
	if err != nil {
		return nodepool.NodeStatusFail
	}
	if resp == nil {
		return nodepool.NodeStatusFail
	}

	body, ok := resp.Data.(map[string]any)
	if !ok {
		return nodepool.NodeStatusSuccess
	}

	// 有 error 字段，需要进一步区分
	if errMsg, exists := body["error"]; exists {
		msg := fmt.Sprintf("%v", errMsg)

		// 限流 / 超频 / 服务过载 → 节点故障，重试其他节点，同时给该节点扣分
		// 这就是你说的"接口通了但超过请求频率"的场景
		if strings.Contains(msg, "rate limit") ||
			strings.Contains(msg, "exceeded") ||
			strings.Contains(msg, "too many requests") {
			return nodepool.NodeStatusFail
		}

		// 节点内部错误 → 也算节点故障
		if strings.Contains(msg, "internal error") {
			return nodepool.NodeStatusFail
		}

		// 其他业务错误（参数不对、地址无效等）→ 换节点也没用，直接返回
		// 不扣节点的分，因为这是调用方的问题
		return nodepool.NodeStatusBizError
	}

	// 有 result 字段，正常成功
	if _, hasResult := body["result"]; hasResult {
		return nodepool.NodeStatusSuccess
	}

	// 格式不对，认为节点有问题
	return nodepool.NodeStatusFail
}

// ============================================================
// 节点配置
// ============================================================

var defaultNodes = []nodepool.NodeConfig{
	{
		Endpoint: "https://node1.example.com",
		Weight:   10, // 权重高，初始优先级高
	},
	{
		Endpoint: "https://node2.example.com",
		Weight:   5,
	},
	{
		Endpoint: "https://node3.example.com",
		Weight:   3,
	},
}

// ============================================================
// 示例 1：Conservative 模式（省额度模式）
// 特点：按节点评分逐个尝试，同一时刻一个请求只在一个节点上执行
// 适用：链上写操作、有配额限制的 API，需要并发安全
// ============================================================

func Example_conservativeMode() {
	// 创建 pool，只需创建一次，可被所有 goroutine 共享
	pool, err := nodepool.New(
		&MockTransport{},
		&RPCClassifier{},
		defaultNodes,
		nodepool.WithStrategy(nodepool.StrategyConservative),
		nodepool.WithMaxRetries(3),                   // 最多重试3轮（每轮尝试所有节点）
		nodepool.WithFailureThreshold(5),             // 连续5次失败触发熔断
		nodepool.WithSuccessThreshold(3),             // 熔断后连续3次成功才恢复
		nodepool.WithCircuitOpenTime(10*time.Second), // 熔断持续10秒
		nodepool.WithSortPeriod(3*time.Second),       // 每3秒重新按分数排序节点
		nodepool.WithEWMAAlpha(0.3),                  // 延迟平滑系数，0.3表示新数据占30%权重
	)
	if err != nil {
		panic(err)
	}
	defer pool.Close() // 停止后台排序协程

	ctx := context.Background()

	// --- 单次请求 ---
	resp, err := pool.Do(ctx, &nodepool.Request{
		Data: map[string]any{
			"method": "getBalance",
			"params": []any{"0xabc123"},
		},
	})
	if err != nil {
		fmt.Printf("请求失败: %v\n", err)
		return
	}
	result := resp.Data.(map[string]any)
	fmt.Printf("结果来自: %s\n", result["endpoint"])
	// 内部流程：
	// 1. 拿当前节点排序快照（读锁，立刻释放）
	// 2. 选评分最高的 node1
	// 3. 调用 MockTransport.Execute(node1, req)
	// 4. Classifier.Classify 判断返回值
	//    - 如果 node1 限流 → NodeStatusFail → 选 node2 重试
	//    - 如果正常 → NodeStatusSuccess → 返回
	// 5. 更新 node1 的健康统计（写锁，纳秒级）
}

// ============================================================
// 示例 2：Race 模式（快速模式）
// 特点：同时发给多个节点，第一个成功的返回，其余取消
// 适用：只读查询，不关心额度消耗，追求最低延迟
// ============================================================

func Example_raceMode() {
	pool, err := nodepool.New(
		&MockTransport{},
		&RPCClassifier{},
		defaultNodes,
		nodepool.WithStrategy(nodepool.StrategyRace),
		nodepool.WithRaceConcurrency(3), // 同时发给最多3个节点，0=全部
	)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	start := time.Now()
	resp, err := pool.Do(context.Background(), &nodepool.Request{
		Data: map[string]any{
			"method": "getBlockNumber",
		},
	})
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("请求失败: %v\n", err)
		return
	}
	result := resp.Data.(map[string]any)
	fmt.Printf("最快节点: %s，耗时: %v\n", result["endpoint"], elapsed)
	// 内部流程：
	// 1. 同时启动3个子 goroutine，各自调用不同节点
	// 2. 通过 context.WithCancel 关联所有子 goroutine
	// 3. 第一个成功的结果写入 channel
	// 4. 主 goroutine 收到结果，调用 cancel() 取消其他节点的请求
	// 5. 其他 goroutine 的 Transport.Execute 收到 ctx.Done()，立刻返回
	// node3 延迟10ms最短，大概率最先返回
}

// ============================================================
// 示例 3：并发安全 — 多 goroutine 共享同一个 Pool
// Pool 创建一次，可以被任意多个 goroutine 同时调用
// 不需要加锁，不需要每个 goroutine 创建自己的 pool
// ============================================================

func Example_concurrentUsage() {
	// 全局只创建一个 pool（通常在 init() 或依赖注入时创建）
	pool, err := nodepool.New(
		&MockTransport{},
		&RPCClassifier{},
		defaultNodes,
		nodepool.WithStrategy(nodepool.StrategyConservative),
		nodepool.WithMaxRetries(3),
	)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	var wg sync.WaitGroup
	successCount := int64(0)
	failCount := int64(0)
	var mu sync.Mutex

	// 模拟 100 个并发请求，全部共享同一个 pool
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 每个 goroutine 直接调用 pool.Do，无需任何额外同步
			// pool 内部通过 RWMutex 保证并发安全：
			//   - Do() 只在"取节点快照"时短暂持读锁（微秒级），
			//     取完立刻释放，之后整个网络请求过程完全无锁
			//   - 多个 goroutine 可以同时持读锁，互不阻塞
			resp, err := pool.Do(context.Background(), &nodepool.Request{
				Data: map[string]any{
					"method": "getBalance",
					"params": []any{fmt.Sprintf("0xaddr_%d", id)},
				},
			})

			mu.Lock()
			if err != nil {
				failCount++
			} else {
				_ = resp
				successCount++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	fmt.Printf("成功: %d, 失败: %d\n", successCount, failCount)

	// 查看每个节点的运行状态
	for _, s := range pool.NodeStats() {
		fmt.Printf("节点: %-35s | 总请求: %3d | 成功: %3d | 失败: %3d | 平均延迟: %v | 熔断: %v\n",
			s.Endpoint, s.TotalRequests, s.SuccessCount, s.FailCount, s.AvgLatency, s.CircuitOpen)
	}
}

// ============================================================
// 示例 4：批量请求 DoMulti
// 多个独立请求并发执行，每个请求独立选节点、独立重试
// ============================================================

func Example_doMulti() {
	pool, err := nodepool.New(
		&MockTransport{},
		&RPCClassifier{},
		defaultNodes,
	)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	// 准备多个独立请求
	addresses := []string{"0xabc", "0xdef", "0x123", "0x456", "0x789"}
	reqs := make([]*nodepool.Request, len(addresses))
	for i, addr := range addresses {
		reqs[i] = &nodepool.Request{
			Data: map[string]any{
				"method": "getBalance",
				"params": []any{addr},
			},
		}
	}

	// DoMulti 内部为每个请求启动一个 goroutine 并发执行
	// 每个请求独立调度：可能去不同的节点，失败了各自重试
	resps, errs := pool.DoMulti(context.Background(), reqs)

	for i, resp := range resps {
		if errs[i] != nil {
			fmt.Printf("地址 %s 查询失败: %v\n", addresses[i], errs[i])
			continue
		}
		result := resp.Data.(map[string]any)
		fmt.Printf("地址 %s → %s\n", addresses[i], result["endpoint"])
	}
}

// ============================================================
// 示例 5：动态节点管理
// 运行时增加、删除、批量同步节点，线程安全
// ============================================================

func Example_dynamicNodes() {
	pool, err := nodepool.New(
		&MockTransport{},
		&RPCClassifier{},
		[]nodepool.NodeConfig{
			{Endpoint: "https://node1.example.com", Weight: 10},
		},
	)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	fmt.Printf("初始节点数: %d\n", pool.NodeCount()) // 1

	// 添加节点（如果 endpoint 已存在，会更新配置）
	pool.AddNode(nodepool.NodeConfig{
		Endpoint: "https://node2.example.com",
		Weight:   5,
	})
	fmt.Printf("添加后节点数: %d\n", pool.NodeCount()) // 2

	// 删除节点
	pool.RemoveNode("https://node1.example.com")
	fmt.Printf("删除后节点数: %d\n", pool.NodeCount()) // 1

	// 批量同步：传入期望的最终状态
	// 内部自动：新增不存在的、更新已存在的、删除不在列表里的
	pool.UpdateNodes([]nodepool.NodeConfig{
		{Endpoint: "https://node2.example.com", Weight: 8}, // 更新权重
		{Endpoint: "https://node3.example.com", Weight: 3}, // 新增
		{Endpoint: "https://node4.example.com", Weight: 1}, // 新增
	})
	fmt.Printf("同步后节点数: %d\n", pool.NodeCount()) // 3

	// 动态管理是并发安全的，可以在请求进行中随时调用
	// pool 内部用写锁保护节点列表，不会和正在执行的请求冲突
}

// ============================================================
// 示例 6：Context 超时控制
// Pool 的请求完全受 context 控制，超时会立刻中断
// ============================================================

func Example_contextTimeout() {
	pool, err := nodepool.New(
		&MockTransport{},
		&RPCClassifier{},
		defaultNodes,
	)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	// 设置 50ms 超时（node1 延迟20ms能完成，node2 延迟50ms刚好超时）
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = pool.Do(ctx, &nodepool.Request{
		Data: map[string]any{"method": "getBalance"},
	})

	if err != nil {
		fmt.Printf("超时了: %v\n", err)
	} else {
		fmt.Println("在超时内完成")
	}
	// context 超时后，正在执行的 Transport.Execute 会立刻收到 ctx.Done()
	// 所有正在进行的子 goroutine（Race 模式）也会被取消
}

// ============================================================
// 示例 7：完整的生产级使用方式
// 把 pool 作为服务的依赖，注入到各处使用
// ============================================================

// BlockchainService 是一个使用 nodepool 的业务服务示例
type BlockchainService struct {
	pool *nodepool.Pool
}

func NewBlockchainService() (*BlockchainService, error) {
	pool, err := nodepool.New(
		&MockTransport{},
		&RPCClassifier{},
		[]nodepool.NodeConfig{
			{Endpoint: "https://rpc1.example.com", Weight: 10},
			{Endpoint: "https://rpc2.example.com", Weight: 5},
			{Endpoint: "https://rpc3.example.com", Weight: 3},
		},
		// 写操作（上链）用 Conservative，保证不重复提交
		nodepool.WithStrategy(nodepool.StrategyConservative),
		nodepool.WithMaxRetries(3),
		nodepool.WithFailureThreshold(5),
		nodepool.WithCircuitOpenTime(30*time.Second),
	)
	if err != nil {
		return nil, err
	}
	return &BlockchainService{pool: pool}, nil
}

func (s *BlockchainService) Close() {
	s.pool.Close()
}

// GetBalance 查询余额（读操作，用 Race 模式更快）
// 可以被多个 goroutine 并发调用，完全安全
func (s *BlockchainService) GetBalance(ctx context.Context, address string) (string, error) {
	resp, err := s.pool.Do(ctx, &nodepool.Request{
		Data: map[string]any{
			"method": "getBalance",
			"params": []any{address},
		},
	})
	if err != nil {
		return "", fmt.Errorf("查询余额失败: %w", err)
	}
	result := resp.Data.(map[string]any)
	return fmt.Sprintf("%v", result["result"]), nil
}

// SendTransaction 发送交易（写操作，用 Conservative 模式保证只发一次）
func (s *BlockchainService) SendTransaction(ctx context.Context, tx map[string]any) (string, error) {
	resp, err := s.pool.Do(ctx, &nodepool.Request{
		Data: map[string]any{
			"method": "sendRawTransaction",
			"params": []any{tx},
		},
	})
	if err != nil {
		return "", fmt.Errorf("发送交易失败: %w", err)
	}
	result := resp.Data.(map[string]any)
	return fmt.Sprintf("%v", result["result"]), nil
}

// BatchGetBalance 批量查询余额，内部并发执行
func (s *BlockchainService) BatchGetBalance(ctx context.Context, addresses []string) (map[string]string, error) {
	reqs := make([]*nodepool.Request, len(addresses))
	for i, addr := range addresses {
		reqs[i] = &nodepool.Request{
			Data: map[string]any{
				"method": "getBalance",
				"params": []any{addr},
			},
		}
	}

	resps, errs := s.pool.DoMulti(ctx, reqs)
	results := make(map[string]string, len(addresses))
	for i, addr := range addresses {
		if errs[i] != nil {
			return nil, fmt.Errorf("查询 %s 失败: %w", addr, errs[i])
		}
		result := resps[i].Data.(map[string]any)
		results[addr] = fmt.Sprintf("%v", result["result"])
	}
	return results, nil
}

func Example_productionUsage() {
	svc, err := NewBlockchainService()
	if err != nil {
		panic(err)
	}
	defer svc.Close()

	ctx := context.Background()

	// 并发调用，共享同一个 pool
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			balance, err := svc.GetBalance(ctx, fmt.Sprintf("0xaddr_%d", id))
			if err != nil {
				fmt.Printf("goroutine-%d 查询失败: %v\n", id, err)
				return
			}
			fmt.Printf("goroutine-%d 余额: %s\n", id, balance)
		}(i)
	}
	wg.Wait()

	// 批量查询
	addresses := []string{"0xaaa", "0xbbb", "0xccc"}
	balances, err := svc.BatchGetBalance(ctx, addresses)
	if err != nil {
		fmt.Printf("批量查询失败: %v\n", err)
		return
	}
	for addr, bal := range balances {
		fmt.Printf("%s → %s\n", addr, bal)
	}

	// 查看节点状态
	fmt.Println("\n节点状态:")
	for _, s := range svc.pool.NodeStats() {
		fmt.Printf("  %s: 总=%d 成功=%d 失败=%d 延迟=%v\n",
			s.Endpoint, s.TotalRequests, s.SuccessCount, s.FailCount, s.AvgLatency)
	}
}
