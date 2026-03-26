package dlock

import (
	"context"
	"sync"
)

// MemoryProvider 是基于内存的 per-key 锁工厂，仅在单进程内有效。
type MemoryProvider struct {
	mu    sync.Mutex
	locks map[string]*memoryLock
}

// memoryLock 是一个引用计数的 per-key 锁内核，多个 memoryMutex 可共享同一个。
type memoryLock struct {
	ch   chan struct{} // cap=1 的 channel 充当互斥信号量
	refs int           // 当前有多少 memoryMutex 引用着此 lock
}

// memoryMutex 实现 Mutex 接口，绑定到一个 memoryLock。
type memoryMutex struct {
	p   *MemoryProvider
	key string
}

// NewMemoryProvider 返回一个内存锁工厂。
func NewMemoryProvider() *MemoryProvider {
	return &MemoryProvider{
		locks: make(map[string]*memoryLock),
	}
}

// NewMutex 创建一个以 key 为标识的内存互斥锁。
func (p *MemoryProvider) NewMutex(key string) Mutex {
	p.mu.Lock()
	defer p.mu.Unlock()

	lk, ok := p.locks[key]
	if !ok {
		lk = &memoryLock{ch: make(chan struct{}, 1)}
		p.locks[key] = lk
	}
	lk.refs++

	return &memoryMutex{p: p, key: key}
}

// getLock 获取 key 对应的 memoryLock（调用方需确保 key 存在）。
func (p *MemoryProvider) getLock(key string) *memoryLock {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.locks[key]
}

// release 减少引用计数，归零时从 map 中移除以避免内存泄漏。
func (p *MemoryProvider) release(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	lk, ok := p.locks[key]
	if !ok {
		return
	}
	lk.refs--
	if lk.refs <= 0 {
		delete(p.locks, key)
	}
}

// Lock 阻塞式获取锁，支持通过 ctx 取消。
func (m *memoryMutex) Lock(ctx context.Context) error {
	lk := m.p.getLock(m.key)
	if lk == nil {
		return ErrLockFailed
	}
	select {
	case lk.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TryLock 非阻塞式尝试获取锁，失败返回 ErrLockFailed。
func (m *memoryMutex) TryLock(_ context.Context) error {
	lk := m.p.getLock(m.key)
	if lk == nil {
		return ErrLockFailed
	}
	select {
	case lk.ch <- struct{}{}:
		return nil
	default:
		return ErrLockFailed
	}
}

// Unlock 释放锁。
func (m *memoryMutex) Unlock(_ context.Context) error {
	lk := m.p.getLock(m.key)
	if lk == nil {
		return nil
	}
	select {
	case <-lk.ch:
	default:
	}
	return nil
}
