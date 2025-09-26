package utils

import (
	"errors"
	"sync"
	"time"
)

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
// waitTime < 0: 一直等 (MustWait)
// waitTime = 0: 不等 (NoWait)
// waitTime > 0: 等待指定时间
func (m *Manager) SingleRun(key string, waitTime time.Duration, f func() error) (RunInfo, error) {
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
			e.mu.Unlock() // 在返回前解锁
			return info, ErrRunning

		// --- Case 2: 无限等待 ---
		case waitTime < 0:
			// 使用 for 循环是 sync.Cond 的标准用法。
			// 因为协程可能被意外唤醒 (spurious wakeup)，所以需要循环检查条件。
			for e.info.IsRunning {
				e.cond.Wait()
			}

		// --- Case 3: 超时等待 (无协程泄漏的正确实现) ---
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
				return info, ErrRunning // 可以考虑返回一个更明确的 "ErrTimeout"
			}
		}
	}

	// 4. 轮到我们执行了，更新状态
	e.info.IsRunning = true
	e.info.StartTime = time.Now()
	e.info.RunCount++

	// 5. 【关键】在执行长时间任务 f() 之前，必须解锁，否则会阻塞其他所有操作
	e.mu.Unlock()

	// 执行任务
	err := f()

	// 6. 任务执行完毕，重新加锁以安全地更新状态
	e.mu.Lock()
	e.info.IsRunning = false
	// 7. 广播通知所有正在等待这个 key 的协程，任务已经完成了
	e.cond.Broadcast()

	info := e.info
	e.mu.Unlock() // 8. 完成所有操作，解锁

	return info, err
}
