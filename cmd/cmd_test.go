// cmd_test.go
package cmd

import (
	"context"
	"sync"
	"testing"
	"time"
)

// CmdHandler 是一个简单的 Handler 实现，
// 每次 Do 被调用就把自己的 Name 记录到全局 slice 里，用于测试计数。
type CmdHandler struct {
	Name string
}

var (
	mu      sync.Mutex
	invoked []string
)

func (h CmdHandler) Spec() string {
	// 100ms 间隔便于测试
	return "@every 1s"
}

func (h CmdHandler) MaxExecuteTime() time.Duration {
	return time.Second
}

func (h CmdHandler) Do(ctx context.Context) error {
	if h.Name == "timeout" {
		time.Sleep(2 * time.Second)
	}
	mu.Lock()
	invoked = append(invoked, h.Name)
	mu.Unlock()
	return nil
}

// resetInvocation 清空全局记录
func resetInvocation() {
	mu.Lock()
	defer mu.Unlock()
	invoked = invoked[:0]
}

// countInvocation 读取当前记录长度
func countInvocation() int {
	mu.Lock()
	defer mu.Unlock()
	return len(invoked)
}

// TestScheduler_TimeoutBehavior 验证当 Do 执行超过 MaxExecuteTime 时，
// 在下一次调度间隔到来前不会并发执行第二次，短窗口内只会产生一次调用。
func TestScheduler_TimeoutBehavior(t *testing.T) {
	// 准备调度器
	resetInvocation()
	sched := NewScheduler()
	if err := sched.AddJob("timeout-job", CmdHandler{"timeout"}); err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}
	sched.Start()
	defer func() { <-sched.Stop().Done() }()

	time.Sleep(4 * time.Second)
	if got := countInvocation(); got != 1 {
		t.Errorf("expected exactly 1 invocation during timeout window, got %d", got)
	}
}

func TestScheduler_DynamicAddRemoveRuntime(t *testing.T) {
	sched := NewScheduler()
	sched.Start()
	defer func() {
		<-sched.Stop().Done()
	}()

	// 1) 动态添加任务
	resetInvocation()
	if err := sched.AddJob("job1", CmdHandler{Name: "job1"}); err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	// 等 250ms，应当触发至少 2 次
	time.Sleep(2 * time.Second)
	if got := countInvocation(); got < 2 {
		t.Errorf("after AddJob, expected ≥2 invocations, got %d", got)
	}

	// 2) 删除任务后不应再有调用
	before := countInvocation()
	if err := sched.RemoveJob("job1"); err != nil {
		t.Fatalf("RemoveJob failed: %v", err)
	}
	// 再等 200ms
	time.Sleep(2 * time.Second)
	if after := countInvocation(); after != before {
		t.Errorf("after RemoveJob, no more runs expected; before=%d, after=%d", before, after)
	}

	// 3) 再次添加（同 ID），应重新触发
	resetInvocation()
	if err := sched.AddJob("job1", CmdHandler{Name: "job1-v2"}); err != nil {
		t.Fatalf("Re-AddJob failed: %v", err)
	}
	time.Sleep(2 * time.Second)
	if got := countInvocation(); got < 2 {
		t.Errorf("after Re-AddJob, expected ≥2 invocations, got %d", got)
	}
}
