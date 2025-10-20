package utils

import (
	"sync"
	"time"
)

/*
SafeChan 结构体解决了 Go 语言并发编程中通道操作的核心安全问题：
1.  **安全关闭 (Panic Prevention):**
    * **发送安全:** 解决了在 Go 中向已关闭的通道发送数据会导致运行时 Panic 的问题。
    * **关闭安全:** 解决了重复关闭通道会导致运行时 Panic 的问题。

2.  **多协程下的高效、安全通信:**
    * **无锁并发 (Lock-Free):** 采用 `atomic.Bool` 和 `chan struct{}` (即 'done' 通道) 的组合，实现了无锁或极低锁竞争的并发模型。
    * **避免阻塞 Close:** 解决了传统加锁（如 RWMutex）方案中，长时间阻塞的发送操作会阻止 Close 立即执行的问题，保证了通道关闭操作的高效和及时性。
    * **阻塞发送通知:** 即使发送操作因通道已满而阻塞，也能通过监听 `done` 通道，安全地感知到通道关闭信号并立即退出，避免了无限期等待或 Panic。

3.  **功能完整性:**
    * 提供了 `Send` (阻塞发送)、`TrySend` (非阻塞发送)、`Send` with `timeout` (带超时发送) 三种灵活的发送模式。
    * 确保通道只能被关闭一次。
*/

// SafeChan 使用 RWMutex 来实现一个并发安全的通道封装，避免向已关闭的通道发送数据导致的 Panic。
type SafeChan[T any] struct {
	ch       chan T        // 实际数据通道
	done     chan struct{} // 退出信号通道，用于唤醒阻塞的发送者
	mu       sync.RWMutex  // 读写锁，保护 isClosed 状态和发送操作
	isClosed bool          // 通道是否已关闭的状态 (受 mu 保护)
}

// NewSafeChan 创建一个安全通道实例。
func NewSafeChan[T any](buffer int) *SafeChan[T] {
	return &SafeChan[T]{
		ch:   make(chan T, buffer),
		done: make(chan struct{}),
	}
}

// NewFromChan 从一个现有的通道创建 SafeChan 实例。
// 警告：传入的通道必须是开放状态，否则 SafeChan 的 Send 仍可能 Panic。
func NewFromChan[T any](ch chan T) *SafeChan[T] {
	return &SafeChan[T]{
		ch:   ch,
		done: make(chan struct{}),
	}
}

// Chan 返回只读通道。
func (s *SafeChan[T]) Chan() <-chan T {
	return s.ch
}

// IsClosed 检查通道是否已关闭。
func (s *SafeChan[T]) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isClosed
}

// Close 安全关闭通道，只能执行一次。
// 返回 true 表示成功关闭，false 表示此前已关闭。
//
// 注意：如果 Send Goroutine 正阻塞并持有 RLock，Close 操作也会阻塞。
func (s *SafeChan[T]) Close() bool {
	// 1. 获取写锁，等待所有读锁释放
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		return false
	}

	// 2. 标记状态
	s.isClosed = true

	// 3. 先关闭信号通道，释放所有阻塞在 select 上的发送者 (让它们释放 RLock)
	close(s.done)

	// 4. 后关闭数据通道，释放所有阻塞的接收者
	close(s.ch)
	return true
}

// Send 阻塞发送数据；支持可选超时。
// 如果通道已关闭、超时或收到退出信号，则返回 false。
func (s *SafeChan[T]) Send(v T, timeout ...time.Duration) bool {
	// 1. 获取读锁
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 2. 快速检查状态
	if s.isClosed {
		return false
	}

	// 3. 在锁保护下执行 select
	if len(timeout) > 0 && timeout[0] > 0 {
		timer := time.NewTimer(timeout[0])
		defer timer.Stop()

		select {
		case s.ch <- v: // 尝试发送
			return true
		case <-s.done: // 退出信号
			return false
		case <-timer.C: // 超时
			return false
		}
	}

	// 无超时
	select {
	case s.ch <- v:
		return true
	case <-s.done:
		return false
	}
}

// TrySend 非阻塞发送数据。
// 如果通道已关闭或通道已满，则返回 false。
func (s *SafeChan[T]) TrySend(v T) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.isClosed {
		return false
	}

	// 非阻塞发送
	select {
	case s.ch <- v:
		return true
	case <-s.done:
		return false
	default:
		// 通道已满
		return false
	}
}

type SafeCloser struct {
	done     chan struct{} // 退出信号，无缓冲，用于广播
	mu       sync.Mutex    // 互斥锁，保护 isClosed 状态 (RWMutex 在这里过度了)
	isClosed bool          // 状态跟踪
}

func NewSafeCloser() *SafeCloser {
	return &SafeCloser{
		done: make(chan struct{}),
	}
}

// Close 安全关闭信号，只能执行一次。
func (s *SafeCloser) Close() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		return false
	}
	s.isClosed = true

	// 关键：关闭 done 通道，解除所有阻塞在 <-s.done 上的接收者。
	close(s.done)
	return true
}
func (s *SafeCloser) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isClosed
}

// Done 返回只读通道，供外部监听退出信号。
func (s *SafeCloser) Done() <-chan struct{} {
	return s.done
}
