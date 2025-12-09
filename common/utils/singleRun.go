package utils

import (
	"context"
	"errors"
	"sync"
	"time"
)

// heldLocksKey 用于在 context 中存储已持有的锁 key 集合
type heldLocksKey struct{}

// getHeldLocks 从 context 中获取已持有的锁 key 集合
func getHeldLocks(ctx context.Context) map[string]bool {
	if ctx == nil {
		return nil
	}
	if locks, ok := ctx.Value(heldLocksKey{}).(map[string]bool); ok {
		return locks
	}
	return nil
}

// withHeldLock 将当前 key 添加到 context 的已持有锁集合中
func withHeldLock(ctx context.Context, key string) context.Context {
	oldLocks := getHeldLocks(ctx)
	// 创建新的 map，避免修改原有的
	newLocks := make(map[string]bool, len(oldLocks)+1)
	for k, v := range oldLocks {
		newLocks[k] = v
	}
	newLocks[key] = true
	return context.WithValue(ctx, heldLocksKey{}, newLocks)
}

var DefaultManager = NewManager()

// RunInfo 存储了任务的运行状态
type RunInfo struct {
	IsRunning bool
	StartTime time.Time
	RunCount  int64
}

// ErrRunning 表示任务已在运行
var ErrRunning = errors.New("process is running")

const (
	// MustWait 表示无限期等待
	MustWait = -1
	// NoWait 表示不等待，立即返回
	NoWait = 0
)

// entry 结构体将每个 key 需要的状态和锁打包在一起
type entry struct {
	mu   sync.Mutex
	cond *sync.Cond
	info RunInfo
}

// Manager 负责管理所有的 entry，实现 Per-Key Lock
type Manager struct {
	mu      sync.Mutex // 这个锁只用来保护下面的 map
	entries map[string]*entry
}

// NewManager 创建一个新的 Manager 实例
func NewManager() *Manager {
	return &Manager{
		entries: make(map[string]*entry),
	}
}

// getEntry 获取或创建一个 key 对应的 entry
// 这是实现 Per-Key Lock 的核心
func (m *Manager) getEntry(key string) *entry {
	m.mu.Lock() // 加锁以保护 map
	defer m.mu.Unlock()

	e, ok := m.entries[key]
	if !ok {
		e = &entry{}
		// 关键：Condition Variable 必须绑定到 entry 自己的锁上
		e.cond = sync.NewCond(&e.mu)
		m.entries[key] = e
	}
	return e
}

// SingleRun 确保对于给定的 key，函数 f 一次只运行一个实例。
// 支持锁的嵌套调用：如果在同一个调用链中（通过 ctx 传递），对同一个 key 的嵌套调用
// 会直接执行 f，而不会尝试重新获取锁，从而避免死锁。
//
// waitTime < 0: 一直等 (MustWait)
// waitTime = 0: 不等 (NoWait)
// waitTime > 0: 等待指定时间
//
// 使用示例：
//
//	manager.SingleRun(ctx, "key", NoWait, func(ctx context.Context) error {
//	    // 嵌套调用同一个 key，不会死锁
//	    return manager.SingleRun(ctx, "key", NoWait, func(ctx context.Context) error {
//	        return nil
//	    })
//	})
func (m *Manager) SingleRun(ctx context.Context, key string, waitTime time.Duration, f func(ctx context.Context) error) (RunInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// 检查是否是嵌套调用（当前调用链已持有该 key 的锁）
	heldLocks := getHeldLocks(ctx)
	if heldLocks != nil && heldLocks[key] {
		// 嵌套调用：直接执行 f，不再获取锁
		// 返回一个表示嵌套调用的 RunInfo
		newCtx := withHeldLock(ctx, key)
		err := f(newCtx)
		return RunInfo{IsRunning: true}, err
	}

	// 1. 获取这个 key 专属的 entry (包含它自己的锁、状态和条件变量)
	e := m.getEntry(key)

	// 2. 锁住这个 key 专属的锁
	e.mu.Lock()

	// 3. 检查任务是否已经在运行
	if e.info.IsRunning {
		switch {
		// --- Case 1: 不等待 ---
		case waitTime == NoWait:
			info := e.info
			e.mu.Unlock()
			return info, ErrRunning

		// --- Case 2: 无限等待 ---
		case waitTime < 0:
			// 使用 for 循环是 sync.Cond 的标准用法。
			// 因为协程可能被意外唤醒 (spurious wakeup)，所以需要循环检查条件。
			for e.info.IsRunning {
				e.cond.Wait()
			}

		// --- Case 3: 超时等待 ---
		case waitTime > 0:
			var timedOut bool
			// 使用 AfterFunc 启动一个定时器。它不会阻塞，只会在时间到了之后
			// 在自己的协程中执行一个函数。
			timer := time.AfterFunc(waitTime, func() {
				// 时间到了，我们需要唤醒等待的协程让它自己发现已经超时。
				e.mu.Lock()
				timedOut = true
				e.cond.Broadcast() // 唤醒所有等待者（包括我们自己）
				e.mu.Unlock()
			})

			// 循环等待，直到任务完成 (IsRunning变为false) 或者超时标志被设置
			for e.info.IsRunning && !timedOut {
				e.cond.Wait()
			}

			// 无论 cond.Wait 是如何返回的，我们都需要停止定时器以释放资源。
			// 如果定时器已经触发，Stop() 会返回 false，这没关系。
			timer.Stop()

			// 如果是因为超时而跳出循环，解锁并返回错误。
			if timedOut {
				info := e.info
				e.mu.Unlock()
				return info, ErrRunning
			}
		}
	}

	// 4. 轮到我们执行了，更新状态
	e.info.IsRunning = true
	e.info.StartTime = time.Now()
	e.info.RunCount++

	// 5. 【关键】在执行长时间任务 f() 之前，必须解锁，否则会阻塞其他所有操作
	e.mu.Unlock()

	// 创建新的 context，记录当前已持有此 key 的锁
	newCtx := withHeldLock(ctx, key)

	// 执行任务
	err := f(newCtx)

	// 6. 任务执行完毕，重新加锁以安全地更新状态
	e.mu.Lock()
	e.info.IsRunning = false
	// 7. 广播通知所有正在等待这个 key 的协程，任务已经完成了
	e.cond.Broadcast()

	info := e.info
	e.mu.Unlock() // 8. 完成所有操作，解锁

	return info, err
}
