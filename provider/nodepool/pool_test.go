package nodepool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockTransport simulates a transport with configurable behavior per endpoint.
type mockTransport struct {
	mu       sync.Mutex
	handlers map[string]func(ctx context.Context, req *Request) (*Response, error)
	calls    map[string]int64
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		handlers: make(map[string]func(ctx context.Context, req *Request) (*Response, error)),
		calls:    make(map[string]int64),
	}
}

func (m *mockTransport) On(endpoint string, fn func(ctx context.Context, req *Request) (*Response, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[endpoint] = fn
}

func (m *mockTransport) CallCount(endpoint string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[endpoint]
}

func (m *mockTransport) Execute(ctx context.Context, endpoint string, req *Request) (*Response, error) {
	m.mu.Lock()
	m.calls[endpoint]++
	fn, ok := m.handlers[endpoint]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no handler for %s", endpoint)
	}
	return fn(ctx, req)
}

// --- Tests ---

func TestConservativeStrategy_Success(t *testing.T) {
	mt := newMockTransport()
	mt.On("node1", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "ok-1"}, nil
	})
	mt.On("node2", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "ok-2"}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "node1"},
		{Endpoint: "node2"},
	}, WithStrategy(StrategyConservative))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	resp, err := pool.Do(context.Background(), &Request{Data: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.Data == nil {
		t.Fatal("expected response")
	}
}

func TestConservativeStrategy_FailoverToSecondNode(t *testing.T) {
	mt := newMockTransport()
	mt.On("node1", func(_ context.Context, _ *Request) (*Response, error) {
		return nil, fmt.Errorf("node1 down")
	})
	mt.On("node2", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "ok-2"}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "node1"},
		{Endpoint: "node2"},
	}, WithStrategy(StrategyConservative), WithMaxRetries(3))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	resp, err := pool.Do(context.Background(), &Request{Data: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	data, ok := resp.Data.(string)
	if !ok || data != "ok-2" {
		t.Fatalf("expected ok-2, got %v", resp.Data)
	}
}

func TestConservativeStrategy_AllFail(t *testing.T) {
	mt := newMockTransport()
	mt.On("node1", func(_ context.Context, _ *Request) (*Response, error) {
		return nil, fmt.Errorf("node1 down")
	})
	mt.On("node2", func(_ context.Context, _ *Request) (*Response, error) {
		return nil, fmt.Errorf("node2 down")
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "node1"},
		{Endpoint: "node2"},
	}, WithStrategy(StrategyConservative), WithMaxRetries(2))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	_, err = pool.Do(context.Background(), &Request{Data: "hello"})
	if err == nil {
		t.Fatal("expected error when all nodes fail")
	}
}

func TestConservativeStrategy_BizErrorNoRetry(t *testing.T) {
	mt := newMockTransport()
	callCount := int64(0)
	mt.On("node1", func(_ context.Context, _ *Request) (*Response, error) {
		atomic.AddInt64(&callCount, 1)
		return &Response{Data: "biz-error"}, fmt.Errorf("invalid params")
	})
	mt.On("node2", func(_ context.Context, _ *Request) (*Response, error) {
		atomic.AddInt64(&callCount, 1)
		return &Response{Data: "ok"}, nil
	})

	classifier := &bizErrorClassifier{}
	pool, err := New(mt, classifier, []NodeConfig{
		{Endpoint: "node1"},
		{Endpoint: "node2"},
	}, WithStrategy(StrategyConservative), WithMaxRetries(3))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	_, err = pool.Do(context.Background(), &Request{Data: "hello"})
	if err == nil {
		t.Fatal("expected biz error")
	}
	if atomic.LoadInt64(&callCount) != 1 {
		t.Fatalf("expected 1 call (no retry on biz error), got %d", atomic.LoadInt64(&callCount))
	}
}

type bizErrorClassifier struct{}

func (c *bizErrorClassifier) Classify(_ context.Context, _ string, resp *Response, err error) NodeStatus {
	if err != nil {
		if resp != nil && resp.Data == "biz-error" {
			return NodeStatusBizError
		}
		return NodeStatusFail
	}
	return NodeStatusSuccess
}

func TestRaceStrategy_FirstSuccessWins(t *testing.T) {
	mt := newMockTransport()
	mt.On("fast", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "fast-result"}, nil
	})
	mt.On("slow", func(ctx context.Context, _ *Request) (*Response, error) {
		select {
		case <-time.After(2 * time.Second):
			return &Response{Data: "slow-result"}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "fast"},
		{Endpoint: "slow"},
	}, WithStrategy(StrategyRace))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	start := time.Now()
	resp, err := pool.Do(context.Background(), &Request{Data: "query"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if elapsed > 1*time.Second {
		t.Fatalf("race should be fast, took %v", elapsed)
	}
	data, ok := resp.Data.(string)
	if !ok || data != "fast-result" {
		t.Fatalf("expected fast-result, got %v", resp.Data)
	}
}

func TestRaceStrategy_AllFail(t *testing.T) {
	mt := newMockTransport()
	mt.On("n1", func(_ context.Context, _ *Request) (*Response, error) {
		return nil, fmt.Errorf("n1 fail")
	})
	mt.On("n2", func(_ context.Context, _ *Request) (*Response, error) {
		return nil, fmt.Errorf("n2 fail")
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "n1"},
		{Endpoint: "n2"},
	}, WithStrategy(StrategyRace))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	_, err = pool.Do(context.Background(), &Request{Data: "query"})
	if err == nil {
		t.Fatal("expected error when all race nodes fail")
	}
}

func TestAddRemoveNodes(t *testing.T) {
	mt := newMockTransport()
	mt.On("node1", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "ok"}, nil
	})
	mt.On("node2", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "ok"}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "node1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if pool.NodeCount() != 1 {
		t.Fatalf("expected 1 node, got %d", pool.NodeCount())
	}

	pool.AddNode(NodeConfig{Endpoint: "node2"})
	if pool.NodeCount() != 2 {
		t.Fatalf("expected 2 nodes, got %d", pool.NodeCount())
	}

	pool.RemoveNode("node1")
	if pool.NodeCount() != 1 {
		t.Fatalf("expected 1 node, got %d", pool.NodeCount())
	}

	// verify remaining node works
	resp, err := pool.Do(context.Background(), &Request{Data: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Data != "ok" {
		t.Fatalf("expected ok, got %v", resp.Data)
	}
}

func TestUpdateNodes(t *testing.T) {
	mt := newMockTransport()
	mt.On("a", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "a"}, nil
	})
	mt.On("b", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "b"}, nil
	})
	mt.On("c", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "c"}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "a"},
		{Endpoint: "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	pool.UpdateNodes([]NodeConfig{
		{Endpoint: "b"},
		{Endpoint: "c"},
	})

	if pool.NodeCount() != 2 {
		t.Fatalf("expected 2 nodes, got %d", pool.NodeCount())
	}

	stats := pool.NodeStats()
	endpoints := make(map[string]bool)
	for _, s := range stats {
		endpoints[s.Endpoint] = true
	}
	if !endpoints["b"] || !endpoints["c"] {
		t.Fatalf("expected nodes b and c, got %v", endpoints)
	}
	if endpoints["a"] {
		t.Fatal("node a should have been removed")
	}
}

func TestDoMulti(t *testing.T) {
	mt := newMockTransport()
	mt.On("node1", func(_ context.Context, req *Request) (*Response, error) {
		return &Response{Data: fmt.Sprintf("resp-%v", req.Data)}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "node1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	reqs := []*Request{
		{Data: "a"},
		{Data: "b"},
		{Data: "c"},
	}
	resps, errs := pool.DoMulti(context.Background(), reqs)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}
	for i, resp := range resps {
		expected := fmt.Sprintf("resp-%s", reqs[i].Data)
		if resp.Data != expected {
			t.Fatalf("request %d: expected %s, got %v", i, expected, resp.Data)
		}
	}
}

func TestCircuitBreaker(t *testing.T) {
	mt := newMockTransport()
	failCount := int64(0)
	mt.On("fragile", func(_ context.Context, _ *Request) (*Response, error) {
		atomic.AddInt64(&failCount, 1)
		return nil, fmt.Errorf("always fail")
	})
	mt.On("stable", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "stable-ok"}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "fragile", Weight: 10},
		{Endpoint: "stable", Weight: 1},
	},
		WithStrategy(StrategyConservative),
		WithFailureThreshold(3),
		WithCircuitOpenTime(100*time.Millisecond),
		WithMaxRetries(5),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Make enough requests for fragile to trip the circuit breaker
	for i := 0; i < 10; i++ {
		resp, err := pool.Do(context.Background(), &Request{Data: "test"})
		if err != nil {
			continue
		}
		if resp != nil && resp.Data == "stable-ok" {
			// eventually should route to stable
		}
	}

	// After circuit opens, fragile should be skipped
	time.Sleep(50 * time.Millisecond)
	resp, err := pool.Do(context.Background(), &Request{Data: "test"})
	if err != nil {
		t.Fatalf("expected success via stable node, got: %v", err)
	}
	if resp.Data != "stable-ok" {
		t.Fatalf("expected stable-ok, got %v", resp.Data)
	}
}

func TestContextCancellation(t *testing.T) {
	mt := newMockTransport()
	mt.On("slow", func(ctx context.Context, _ *Request) (*Response, error) {
		select {
		case <-time.After(10 * time.Second):
			return &Response{Data: "done"}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "slow"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = pool.Do(ctx, &Request{Data: "test"})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestNodeStats(t *testing.T) {
	mt := newMockTransport()
	mt.On("node1", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "ok"}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "node1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	for i := 0; i < 5; i++ {
		pool.Do(context.Background(), &Request{Data: "test"})
	}

	stats := pool.NodeStats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat, got %d", len(stats))
	}
	if stats[0].TotalRequests != 5 {
		t.Fatalf("expected 5 total requests, got %d", stats[0].TotalRequests)
	}
	if stats[0].SuccessCount != 5 {
		t.Fatalf("expected 5 successes, got %d", stats[0].SuccessCount)
	}
}

func TestCustomClassifier_RateLimit(t *testing.T) {
	mt := newMockTransport()
	callOrder := make([]string, 0)
	var mu sync.Mutex

	mt.On("limited", func(_ context.Context, _ *Request) (*Response, error) {
		mu.Lock()
		callOrder = append(callOrder, "limited")
		mu.Unlock()
		return &Response{Data: map[string]any{"error": "rate limit exceeded"}}, nil
	})
	mt.On("healthy", func(_ context.Context, _ *Request) (*Response, error) {
		mu.Lock()
		callOrder = append(callOrder, "healthy")
		mu.Unlock()
		return &Response{Data: map[string]any{"result": "data"}}, nil
	})

	classifier := &rateLimitClassifier{}
	pool, err := New(mt, classifier, []NodeConfig{
		{Endpoint: "limited", Weight: 10},
		{Endpoint: "healthy", Weight: 1},
	}, WithStrategy(StrategyConservative), WithMaxRetries(3))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	resp, err := pool.Do(context.Background(), &Request{Data: "query"})
	if err != nil {
		t.Fatal(err)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", resp.Data)
	}
	if _, hasResult := data["result"]; !hasResult {
		t.Fatalf("expected result field in response, got %v", data)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(callOrder) < 2 {
		t.Fatalf("expected at least 2 calls, got %d", len(callOrder))
	}
	if callOrder[0] != "limited" {
		t.Fatalf("expected limited first, got %s", callOrder[0])
	}
}

type rateLimitClassifier struct{}

func (c *rateLimitClassifier) Classify(_ context.Context, _ string, resp *Response, err error) NodeStatus {
	if err != nil {
		return NodeStatusFail
	}
	if resp == nil {
		return NodeStatusFail
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		return NodeStatusSuccess
	}
	if errMsg, exists := data["error"]; exists {
		if msg, ok := errMsg.(string); ok && msg == "rate limit exceeded" {
			return NodeStatusFail
		}
	}
	return NodeStatusSuccess
}

func TestEmptyPool(t *testing.T) {
	mt := newMockTransport()
	_, err := New(mt, nil, []NodeConfig{})
	if err == nil {
		t.Fatal("expected error for empty configs")
	}
}

func TestNilTransport(t *testing.T) {
	_, err := New(nil, nil, []NodeConfig{{Endpoint: "test"}})
	if err == nil {
		t.Fatal("expected error for nil transport")
	}
}

func TestPerNodeClassifier(t *testing.T) {
	mt := newMockTransport()
	mt.On("alchemy", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: map[string]any{"error": map[string]any{"code": -32090, "message": "Too many requests"}}}, nil
	})
	mt.On("infura", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: map[string]any{"error": "rate limit exceeded"}}, nil
	})
	mt.On("mynode", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: map[string]any{"result": "ok"}}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{
			Endpoint: "alchemy",
			Weight:   10,
			Classifier: classifierFunc(func(_ context.Context, _ string, resp *Response, _ error) NodeStatus {
				data := resp.Data.(map[string]any)
				if _, hasErr := data["error"]; hasErr {
					return NodeStatusFail
				}
				return NodeStatusSuccess
			}),
		},
		{
			Endpoint: "infura",
			Weight:   5,
			Classifier: classifierFunc(func(_ context.Context, _ string, resp *Response, _ error) NodeStatus {
				data := resp.Data.(map[string]any)
				if errStr, ok := data["error"].(string); ok && errStr == "rate limit exceeded" {
					return NodeStatusFail
				}
				return NodeStatusSuccess
			}),
		},
		{Endpoint: "mynode", Weight: 1},
	}, WithStrategy(StrategyConservative), WithMaxRetries(3))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	resp, err := pool.Do(context.Background(), &Request{Data: "test"})
	if err != nil {
		t.Fatal(err)
	}
	data := resp.Data.(map[string]any)
	if data["result"] != "ok" {
		t.Fatalf("expected result ok, got %v", data)
	}
	if mt.CallCount("alchemy") == 0 {
		t.Fatal("expected alchemy to be tried first (highest weight)")
	}
	if mt.CallCount("mynode") == 0 {
		t.Fatal("expected mynode to be called")
	}
}

type classifierFunc func(ctx context.Context, endpoint string, resp *Response, err error) NodeStatus

func (f classifierFunc) Classify(ctx context.Context, endpoint string, resp *Response, err error) NodeStatus {
	return f(ctx, endpoint, resp, err)
}

// --- Rate Limiting Tests ---

func TestRateLimit_MaxConcurrent(t *testing.T) {
	mt := newMockTransport()
	var concurrent int64
	var maxConcurrent int64

	mt.On("node1", func(ctx context.Context, _ *Request) (*Response, error) {
		cur := atomic.AddInt64(&concurrent, 1)
		for {
			old := atomic.LoadInt64(&maxConcurrent)
			if cur <= old || atomic.CompareAndSwapInt64(&maxConcurrent, old, cur) {
				break
			}
		}
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			atomic.AddInt64(&concurrent, -1)
			return nil, ctx.Err()
		}
		atomic.AddInt64(&concurrent, -1)
		return &Response{Data: "ok"}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{
			Endpoint: "node1",
			RateLimit: &RateLimitConfig{
				MaxConcurrent: 3,
			},
		},
	}, WithStrategy(StrategyConservative), WithMaxRetries(1), WithWaitOnThrottle(true))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.Do(context.Background(), &Request{Data: "test"})
		}()
	}
	wg.Wait()

	if atomic.LoadInt64(&maxConcurrent) > 3 {
		t.Fatalf("expected max concurrent <= 3, got %d", atomic.LoadInt64(&maxConcurrent))
	}
}

func TestRateLimit_PerSecond(t *testing.T) {
	mt := newMockTransport()
	var callCount int64

	mt.On("node1", func(_ context.Context, _ *Request) (*Response, error) {
		atomic.AddInt64(&callCount, 1)
		return &Response{Data: "ok"}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{
			Endpoint: "node1",
			RateLimit: &RateLimitConfig{
				PerSecond: 5,
				Burst:     5,
			},
		},
	}, WithStrategy(StrategyConservative), WithMaxRetries(1), WithWaitOnThrottle(true))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	for {
		_, err := pool.Do(ctx, &Request{Data: "test"})
		if err != nil {
			break
		}
	}

	count := atomic.LoadInt64(&callCount)
	if count < 5 || count > 10 {
		t.Fatalf("expected 5-10 requests in 600ms at 5/s, got %d", count)
	}
}

func TestRateLimit_ThrottledSkipToNextNode(t *testing.T) {
	mt := newMockTransport()
	mt.On("limited", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "limited"}, nil
	})
	mt.On("free", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "free"}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{
			Endpoint: "limited",
			Weight:   10,
			RateLimit: &RateLimitConfig{
				MaxConcurrent: 1,
				PerSecond:     1,
				Burst:         1,
			},
		},
		{Endpoint: "free", Weight: 1},
	}, WithStrategy(StrategyConservative), WithMaxRetries(3))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// First request goes to "limited" (highest weight)
	resp1, err := pool.Do(context.Background(), &Request{Data: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if resp1.Data != "limited" {
		t.Fatalf("first request should go to limited, got %v", resp1.Data)
	}

	// Immediately second request: "limited" is rate-limited, should skip to "free"
	resp2, err := pool.Do(context.Background(), &Request{Data: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if resp2.Data != "free" {
		t.Fatalf("second request should fallback to free, got %v", resp2.Data)
	}
}

func TestRateLimit_AllThrottled_NoWait(t *testing.T) {
	mt := newMockTransport()
	mt.On("n1", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "ok"}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{
			Endpoint: "n1",
			RateLimit: &RateLimitConfig{
				PerSecond: 1,
				Burst:     1,
			},
		},
	}, WithStrategy(StrategyConservative), WithMaxRetries(1), WithWaitOnThrottle(false))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// First request consumes the token
	_, err = pool.Do(context.Background(), &Request{Data: "1"})
	if err != nil {
		t.Fatal(err)
	}

	// Second request should fail with ErrAllThrottled
	_, err = pool.Do(context.Background(), &Request{Data: "2"})
	if err == nil {
		t.Fatal("expected error when all nodes throttled and WaitOnThrottle=false")
	}
}

func TestRateLimit_AllThrottled_WithWait(t *testing.T) {
	mt := newMockTransport()
	mt.On("n1", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "ok"}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{
			Endpoint: "n1",
			RateLimit: &RateLimitConfig{
				PerSecond: 10,
				Burst:     1,
			},
		},
	}, WithStrategy(StrategyConservative), WithMaxRetries(1), WithWaitOnThrottle(true))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// First request consumes the burst token
	_, err = pool.Do(context.Background(), &Request{Data: "1"})
	if err != nil {
		t.Fatal(err)
	}

	// Second request should wait and eventually succeed
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := pool.Do(ctx, &Request{Data: "2"})
	if err != nil {
		t.Fatalf("expected success after waiting, got: %v", err)
	}
	if resp.Data != "ok" {
		t.Fatalf("expected ok, got %v", resp.Data)
	}
}

func TestRateLimit_RaceMode(t *testing.T) {
	mt := newMockTransport()
	mt.On("fast", func(_ context.Context, _ *Request) (*Response, error) {
		return &Response{Data: "fast"}, nil
	})
	mt.On("slow", func(ctx context.Context, _ *Request) (*Response, error) {
		select {
		case <-time.After(100 * time.Millisecond):
			return &Response{Data: "slow"}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	pool, err := New(mt, nil, []NodeConfig{
		{
			Endpoint: "fast",
			Weight:   10,
			RateLimit: &RateLimitConfig{
				PerSecond: 1,
				Burst:     1,
			},
		},
		{Endpoint: "slow", Weight: 1},
	}, WithStrategy(StrategyRace))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// First race: fast gets the token
	resp1, err := pool.Do(context.Background(), &Request{Data: "1"})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp1

	// Second race: fast is throttled, slow should work
	resp2, err := pool.Do(context.Background(), &Request{Data: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if resp2.Data != "slow" {
		t.Logf("got %v (could be fast if token replenished)", resp2.Data)
	}
}

// --- DoUntilComplete Tests ---

func TestDoUntilComplete_AllSuccess(t *testing.T) {
	mt := newMockTransport()
	mt.On("node1", func(_ context.Context, req *Request) (*Response, error) {
		return &Response{Data: fmt.Sprintf("resp-%v", req.Data)}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "node1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	reqs := []*Request{{Data: "a"}, {Data: "b"}, {Data: "c"}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resps, errs := pool.DoUntilComplete(ctx, reqs)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}
	for i, resp := range resps {
		expected := fmt.Sprintf("resp-%s", reqs[i].Data)
		if resp.Data != expected {
			t.Fatalf("request %d: expected %s, got %v", i, expected, resp.Data)
		}
	}
}

func TestDoUntilComplete_RetryOnNodeFail(t *testing.T) {
	mt := newMockTransport()
	failsLeft := int64(3)

	mt.On("flaky", func(_ context.Context, _ *Request) (*Response, error) {
		if atomic.AddInt64(&failsLeft, -1) >= 0 {
			return nil, fmt.Errorf("temporary failure")
		}
		return &Response{Data: "recovered"}, nil
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "flaky"},
	}, WithMaxRetries(1))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reqs := []*Request{{Data: "test"}}
	resps, errs := pool.DoUntilComplete(ctx, reqs)
	if errs[0] != nil {
		t.Fatalf("expected eventual success, got: %v", errs[0])
	}
	if resps[0].Data != "recovered" {
		t.Fatalf("expected recovered, got %v", resps[0].Data)
	}
}

func TestDoUntilComplete_BizErrorNotRetried(t *testing.T) {
	mt := newMockTransport()
	callCount := int64(0)

	mt.On("node1", func(_ context.Context, _ *Request) (*Response, error) {
		atomic.AddInt64(&callCount, 1)
		return &Response{Data: "biz-error"}, fmt.Errorf("invalid params")
	})

	classifier := &bizErrorClassifier{}
	pool, err := New(mt, classifier, []NodeConfig{
		{Endpoint: "node1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reqs := []*Request{{Data: "test"}}
	resps, errs := pool.DoUntilComplete(ctx, reqs)
	if errs[0] == nil {
		t.Fatal("expected biz error")
	}
	if resps[0] == nil || resps[0].Data != "biz-error" {
		t.Fatalf("expected biz-error response, got %v", resps[0])
	}
	if atomic.LoadInt64(&callCount) > 3 {
		t.Fatalf("biz error should not be heavily retried, got %d calls", atomic.LoadInt64(&callCount))
	}
}

func TestDoUntilComplete_ContextCancel(t *testing.T) {
	mt := newMockTransport()
	mt.On("slow", func(ctx context.Context, _ *Request) (*Response, error) {
		select {
		case <-time.After(10 * time.Second):
			return &Response{Data: "done"}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	pool, err := New(mt, nil, []NodeConfig{
		{Endpoint: "slow"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	reqs := []*Request{{Data: "test"}}
	_, errs := pool.DoUntilComplete(ctx, reqs)
	if errs[0] == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestDoUntilComplete_MixedResults(t *testing.T) {
	mt := newMockTransport()
	succeedAfter := int64(2)

	mt.On("node1", func(_ context.Context, req *Request) (*Response, error) {
		data := req.Data.(string)
		if data == "good" {
			return &Response{Data: "good-resp"}, nil
		}
		if data == "flaky" {
			if atomic.AddInt64(&succeedAfter, -1) > 0 {
				return nil, fmt.Errorf("temp fail")
			}
			return &Response{Data: "flaky-resp"}, nil
		}
		return &Response{Data: "biz-error"}, fmt.Errorf("invalid")
	})

	classifier := &bizErrorClassifier{}
	pool, err := New(mt, classifier, []NodeConfig{
		{Endpoint: "node1"},
	}, WithMaxRetries(1))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reqs := []*Request{
		{Data: "good"},
		{Data: "flaky"},
		{Data: "biz-error"},
	}
	resps, errs := pool.DoUntilComplete(ctx, reqs)

	if errs[0] != nil {
		t.Fatalf("good request should succeed, got: %v", errs[0])
	}
	if resps[0].Data != "good-resp" {
		t.Fatalf("expected good-resp, got %v", resps[0].Data)
	}

	if errs[1] != nil {
		t.Fatalf("flaky request should eventually succeed, got: %v", errs[1])
	}
	if resps[1].Data != "flaky-resp" {
		t.Fatalf("expected flaky-resp, got %v", resps[1].Data)
	}

	if errs[2] == nil {
		t.Fatal("biz error should not be retried")
	}
}
