package dlock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================
// Memory 后端测试（无外部依赖，始终运行）
// ============================================================

func TestMemory_LockUnlock(t *testing.T) {
	p := NewMemoryProvider()
	m := p.NewMutex("k1")

	ctx := context.Background()
	if err := m.Lock(ctx); err != nil {
		t.Fatalf("Lock failed: %v", err)
	}
	if err := m.Unlock(ctx); err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}
}

func TestMemory_TryLock_Success(t *testing.T) {
	p := NewMemoryProvider()
	m := p.NewMutex("k1")

	ctx := context.Background()
	if err := m.TryLock(ctx); err != nil {
		t.Fatalf("TryLock should succeed: %v", err)
	}
	defer m.Unlock(ctx)
}

func TestMemory_TryLock_Fail(t *testing.T) {
	p := NewMemoryProvider()
	m1 := p.NewMutex("k1")
	m2 := p.NewMutex("k1")

	ctx := context.Background()
	if err := m1.Lock(ctx); err != nil {
		t.Fatalf("Lock failed: %v", err)
	}
	defer m1.Unlock(ctx)

	if err := m2.TryLock(ctx); err != ErrLockFailed {
		t.Fatalf("expected ErrLockFailed, got %v", err)
	}
}

func TestMemory_Lock_ContextCancel(t *testing.T) {
	p := NewMemoryProvider()
	m1 := p.NewMutex("k1")
	m2 := p.NewMutex("k1")

	ctx := context.Background()
	if err := m1.Lock(ctx); err != nil {
		t.Fatalf("Lock failed: %v", err)
	}
	defer m1.Unlock(ctx)

	cancelCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	err := m2.Lock(cancelCtx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestMemory_DifferentKeys_Independent(t *testing.T) {
	p := NewMemoryProvider()
	m1 := p.NewMutex("k1")
	m2 := p.NewMutex("k2")

	ctx := context.Background()
	if err := m1.Lock(ctx); err != nil {
		t.Fatalf("Lock k1 failed: %v", err)
	}
	defer m1.Unlock(ctx)

	if err := m2.TryLock(ctx); err != nil {
		t.Fatalf("TryLock k2 should succeed independently: %v", err)
	}
	defer m2.Unlock(ctx)
}

func TestMemory_MutualExclusion(t *testing.T) {
	p := NewMemoryProvider()
	ctx := context.Background()
	var counter int64
	var violations int64

	const goroutines = 10
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				m := p.NewMutex("shared")
				if err := m.Lock(ctx); err != nil {
					t.Errorf("Lock failed: %v", err)
					return
				}
				cur := atomic.AddInt64(&counter, 1)
				if cur != 1 {
					atomic.AddInt64(&violations, 1)
				}
				time.Sleep(time.Microsecond)
				atomic.AddInt64(&counter, -1)
				_ = m.Unlock(ctx)
			}
		}()
	}

	wg.Wait()
	if v := atomic.LoadInt64(&violations); v > 0 {
		t.Fatalf("mutual exclusion violated %d times", v)
	}
}

func TestMemory_ReferenceCount_Cleanup(t *testing.T) {
	p := NewMemoryProvider()
	m := p.NewMutex("temp")

	ctx := context.Background()
	_ = m.Lock(ctx)
	_ = m.Unlock(ctx)

	// release 引用计数
	p.release("temp")

	p.mu.Lock()
	_, exists := p.locks["temp"]
	p.mu.Unlock()

	if exists {
		t.Fatal("expected lock entry to be cleaned up after refs drop to 0")
	}
}

func TestMemory_UnlockWithoutLock(t *testing.T) {
	p := NewMemoryProvider()
	m := p.NewMutex("k1")

	if err := m.Unlock(context.Background()); err != nil {
		t.Fatalf("Unlock on unheld lock should not error: %v", err)
	}
}

// ============================================================
// Provider 接口兼容性编译检查
// ============================================================

var (
	_ Provider = (*MemoryProvider)(nil)
	_ Provider = (*RedisProvider)(nil)
	_ Provider = (*EtcdProvider)(nil)

	_ Mutex = (*memoryMutex)(nil)
	_ Mutex = (*redisMutex)(nil)
	_ Mutex = (*etcdMutex)(nil)
)
