package cmd

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 测试用 Handler 实现
// ---------------------------------------------------------------------------

type testHandler struct {
	name     string
	spec     string
	maxExec  time.Duration
	doFunc   func(ctx context.Context) error
	invoked  atomic.Int64
	lastCtx  atomic.Value // 保存最近一次 Do 收到的 ctx
	sleeping time.Duration
}

func (h *testHandler) Spec() string                  { return h.spec }
func (h *testHandler) MaxExecuteTime() time.Duration { return h.maxExec }
func (h *testHandler) Do(ctx context.Context) error {
	h.lastCtx.Store(ctx)
	if h.sleeping > 0 {
		time.Sleep(h.sleeping)
	}
	if h.doFunc != nil {
		err := h.doFunc(ctx)
		h.invoked.Add(1)
		return err
	}
	h.invoked.Add(1)
	return nil
}

func newHandler(name string) *testHandler {
	return &testHandler{
		name:    name,
		spec:    "@every 1s",
		maxExec: 5 * time.Second,
	}
}

func newTestScheduler(t *testing.T) *Scheduler {
	t.Helper()
	sched, err := NewScheduler()
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}
	return sched
}

// ---------------------------------------------------------------------------
// 基础功能
// ---------------------------------------------------------------------------

func TestNewScheduler(t *testing.T) {
	sched, err := NewScheduler()
	if err != nil {
		t.Fatalf("NewScheduler returned error: %v", err)
	}
	if sched == nil {
		t.Fatal("NewScheduler returned nil")
	}
	_ = sched.Stop()
}

func TestAddJob_Basic(t *testing.T) {
	sched := newTestScheduler(t)
	h := newHandler("basic")
	sched.Start()
	defer func() { _ = sched.Stop() }()

	if err := sched.AddJob("j1", h); err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	time.Sleep(2500 * time.Millisecond)
	if got := h.invoked.Load(); got < 2 {
		t.Errorf("expected ≥2 invocations, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// AddJob 幂等性
// ---------------------------------------------------------------------------

func TestAddJob_Idempotent(t *testing.T) {
	sched := newTestScheduler(t)
	sched.Start()
	defer func() { _ = sched.Stop() }()

	h1 := newHandler("h1")
	h2 := newHandler("h2")

	if err := sched.AddJob("same-id", h1); err != nil {
		t.Fatal(err)
	}
	if err := sched.AddJob("same-id", h2); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2500 * time.Millisecond)

	if h1.invoked.Load() < 2 {
		t.Errorf("h1 should have been invoked ≥2 times, got %d", h1.invoked.Load())
	}
	if h2.invoked.Load() != 0 {
		t.Errorf("h2 should never be invoked (same id already registered), got %d", h2.invoked.Load())
	}
}

// ---------------------------------------------------------------------------
// ForceAddJob 替换
// ---------------------------------------------------------------------------

func TestForceAddJob_ReplacesExisting(t *testing.T) {
	sched := newTestScheduler(t)
	sched.Start()
	defer func() { _ = sched.Stop() }()

	h1 := newHandler("v1")
	h2 := newHandler("v2")

	if err := sched.AddJob("replaceable", h1); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	if h1.invoked.Load() == 0 {
		t.Fatal("h1 should have been invoked at least once")
	}

	if err := sched.ForceAddJob("replaceable", h2); err != nil {
		t.Fatal(err)
	}

	h1Before := h1.invoked.Load()
	time.Sleep(2500 * time.Millisecond)

	if h1.invoked.Load() != h1Before {
		t.Errorf("h1 should stop running after ForceAddJob; before=%d, after=%d", h1Before, h1.invoked.Load())
	}
	if h2.invoked.Load() < 2 {
		t.Errorf("h2 should have been invoked ≥2 times after replacement, got %d", h2.invoked.Load())
	}
}

// ---------------------------------------------------------------------------
// RemoveJob
// ---------------------------------------------------------------------------

func TestRemoveJob_StopsExecution(t *testing.T) {
	sched := newTestScheduler(t)
	sched.Start()
	defer func() { _ = sched.Stop() }()

	h := newHandler("removable")
	if err := sched.AddJob("rm1", h); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)

	before := h.invoked.Load()
	if err := sched.RemoveJob("rm1"); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)
	if h.invoked.Load() != before {
		t.Errorf("no more runs expected after RemoveJob; before=%d, after=%d", before, h.invoked.Load())
	}
}

func TestRemoveJob_NotFound(t *testing.T) {
	sched := newTestScheduler(t)
	defer func() { _ = sched.Stop() }()

	err := sched.RemoveJob("non-existent")
	if err == nil {
		t.Fatal("RemoveJob should return error for non-existent id")
	}
}

// ---------------------------------------------------------------------------
// ListJobs
// ---------------------------------------------------------------------------

func TestListJobs(t *testing.T) {
	sched := newTestScheduler(t)
	sched.Start()
	defer func() { _ = sched.Stop() }()

	_ = sched.AddJob("list-a", newHandler("a"))
	_ = sched.AddJob("list-b", newHandler("b"))

	time.Sleep(200 * time.Millisecond)
	jobs := sched.ListJobs()

	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	for _, id := range []string{"list-a", "list-b"} {
		nextRun, ok := jobs[id]
		if !ok {
			t.Errorf("job %q not found in ListJobs", id)
			continue
		}
		if nextRun.IsZero() {
			t.Errorf("job %q has zero NextRun", id)
		}
	}
}

func TestListJobs_AfterRemove(t *testing.T) {
	sched := newTestScheduler(t)
	sched.Start()
	defer func() { _ = sched.Stop() }()

	_ = sched.AddJob("temp", newHandler("temp"))
	_ = sched.RemoveJob("temp")

	jobs := sched.ListJobs()
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs after removal, got %d", len(jobs))
	}
}

// ---------------------------------------------------------------------------
// SingletonMode：上一次未完成时跳过本次
// ---------------------------------------------------------------------------

func TestSingletonMode_SkipOverlap(t *testing.T) {
	sched := newTestScheduler(t)
	sched.Start()
	defer func() { _ = sched.Stop() }()

	h := newHandler("slow")
	h.sleeping = 3 * time.Second
	h.maxExec = 10 * time.Second

	if err := sched.AddJob("singleton", h); err != nil {
		t.Fatal(err)
	}

	// @every 1s 触发多次，但 Do 需要 3s，SingletonMode 会跳过重叠触发。
	// 5s 窗口内只有第 1 次能完成（t≈1 开始, t≈4 完成），第 2 次正在运行中。
	time.Sleep(5 * time.Second)
	got := h.invoked.Load()
	if got != 1 {
		t.Errorf("expected 1 completed invocation (singleton mode), got %d", got)
	}
}

// ---------------------------------------------------------------------------
// 超时告警：执行时间超过 MaxExecuteTime 时触发
// ---------------------------------------------------------------------------

func TestTimeoutAlert(t *testing.T) {
	sched := newTestScheduler(t)
	sched.Start()
	defer func() { _ = sched.Stop() }()

	h := newHandler("timeout-alert")
	h.sleeping = 2 * time.Second
	h.maxExec = 1 * time.Second

	if err := sched.AddJob("timeout-test", h); err != nil {
		t.Fatal(err)
	}

	// 首次触发约 1s 后，Do 内 sleep 2s，约 3s 后完成。留足余量等 4s。
	// 超时告警通过 e.SendMessage 发送，此处验证任务本身正常完成（不被取消）。
	time.Sleep(4 * time.Second)
	if h.invoked.Load() < 1 {
		t.Error("job should have completed at least once even with timeout alert")
	}
}

// ---------------------------------------------------------------------------
// Do 返回错误时的处理
// ---------------------------------------------------------------------------

func TestDoError_StillScheduled(t *testing.T) {
	sched := newTestScheduler(t)
	sched.Start()
	defer func() { _ = sched.Stop() }()

	h := newHandler("err-job")
	h.doFunc = func(ctx context.Context) error {
		return errors.New("task failed")
	}

	if err := sched.AddJob("err-test", h); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2500 * time.Millisecond)
	if got := h.invoked.Load(); got < 2 {
		t.Errorf("error-returning job should continue to be scheduled, got %d invocations", got)
	}
}

// ---------------------------------------------------------------------------
// Panic Recovery
// ---------------------------------------------------------------------------

func TestPanicRecovery(t *testing.T) {
	sched := newTestScheduler(t)
	sched.Start()
	defer func() { _ = sched.Stop() }()

	var beforePanic, afterPanic atomic.Int64

	h := newHandler("panic-job")
	h.doFunc = func(ctx context.Context) error {
		beforePanic.Add(1)
		if beforePanic.Load() == 1 {
			panic("test panic")
		}
		afterPanic.Add(1)
		return nil
	}

	if err := sched.AddJob("panic-test", h); err != nil {
		t.Fatal(err)
	}

	time.Sleep(3500 * time.Millisecond)
	if beforePanic.Load() < 2 {
		t.Errorf("job should have been triggered ≥2 times, got %d", beforePanic.Load())
	}
	if afterPanic.Load() < 1 {
		t.Errorf("job should have recovered and run again after panic, got %d post-panic runs", afterPanic.Load())
	}
}

// ---------------------------------------------------------------------------
// Stop 优雅关闭
// ---------------------------------------------------------------------------

func TestStop_WaitsForRunningJobs(t *testing.T) {
	sched := newTestScheduler(t)
	sched.Start()

	var finished atomic.Bool
	h := newHandler("long-running")
	h.doFunc = func(ctx context.Context) error {
		time.Sleep(2 * time.Second)
		finished.Store(true)
		return nil
	}

	if err := sched.AddJob("stop-test", h); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1500 * time.Millisecond) // 确保任务已开始执行
	if err := sched.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if !finished.Load() {
		t.Error("Stop should have waited for running job to finish")
	}
}

// ---------------------------------------------------------------------------
// 多任务并发调度
// ---------------------------------------------------------------------------

func TestMultipleJobs_Concurrent(t *testing.T) {
	sched := newTestScheduler(t)
	sched.Start()
	defer func() { _ = sched.Stop() }()

	handlers := make([]*testHandler, 5)
	for i := range handlers {
		handlers[i] = newHandler("")
		id := "concurrent-" + string(rune('A'+i))
		if err := sched.AddJob(id, handlers[i]); err != nil {
			t.Fatalf("AddJob %s failed: %v", id, err)
		}
	}

	time.Sleep(2500 * time.Millisecond)
	for i, h := range handlers {
		if got := h.invoked.Load(); got < 2 {
			t.Errorf("handler[%d] expected ≥2 invocations, got %d", i, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 动态增删完整流程
// ---------------------------------------------------------------------------

func TestDynamicLifecycle(t *testing.T) {
	sched := newTestScheduler(t)
	sched.Start()
	defer func() { _ = sched.Stop() }()

	h1 := newHandler("v1")
	h2 := newHandler("v2")
	h3 := newHandler("v3")

	// 1. 添加
	if err := sched.AddJob("job", h1); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	if h1.invoked.Load() == 0 {
		t.Fatal("h1 should have run")
	}

	// 2. 同 id AddJob 不覆盖
	if err := sched.AddJob("job", h2); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	if h2.invoked.Load() != 0 {
		t.Fatal("h2 should not run, same id already exists")
	}

	// 3. ForceAddJob 替换
	h1Before := h1.invoked.Load()
	if err := sched.ForceAddJob("job", h3); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2500 * time.Millisecond)
	if h1.invoked.Load() != h1Before {
		t.Errorf("h1 should have stopped after ForceAddJob; before=%d, after=%d", h1Before, h1.invoked.Load())
	}
	if h3.invoked.Load() < 2 {
		t.Errorf("h3 should be running, got %d invocations", h3.invoked.Load())
	}

	// 4. 删除
	if err := sched.RemoveJob("job"); err != nil {
		t.Fatal(err)
	}
	h3Before := h3.invoked.Load()
	time.Sleep(2 * time.Second)
	if h3.invoked.Load() != h3Before {
		t.Errorf("h3 should have stopped after RemoveJob; before=%d, after=%d", h3Before, h3.invoked.Load())
	}

	// 5. ListJobs 应为空
	if jobs := sched.ListJobs(); len(jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(jobs))
	}
}

// ---------------------------------------------------------------------------
// 并发安全：多 goroutine 同时操作
// ---------------------------------------------------------------------------

func TestConcurrentAccess(t *testing.T) {
	sched := newTestScheduler(t)
	sched.Start()
	defer func() { _ = sched.Stop() }()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := "race-" + string(rune('A'+idx))
			h := newHandler(id)
			_ = sched.AddJob(id, h)
			time.Sleep(500 * time.Millisecond)
			_ = sched.ListJobs()
			_ = sched.RemoveJob(id)
		}(i)
	}
	wg.Wait()
}
