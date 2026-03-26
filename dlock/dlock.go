// Package dlock 提供统一的分布式锁抽象，支持 Memory / Redis / etcd 三种后端。
//
// # 核心概念
//
//   - Provider：锁工厂，负责创建指定 key 的 Mutex 实例。
//   - Mutex：互斥锁接口，提供 Lock / TryLock / Unlock 三个操作，均支持 context 取消。
//
// # 选型指南
//
//   - Memory：单进程内的 per-key 互斥锁，无需外部依赖，适用于单实例应用或本地测试。
//   - Redis（redsync）：基于 SET NX PX 的分布式锁，性能高、部署广泛；
//     单 Redis 节点时属于 AP 模型，适用于「效率型」场景（防止重复计算）。
//   - etcd：基于 Raft 共识 + Lease 的分布式锁，强一致（CP），
//     适用于「正确性型」场景（防止数据冲突）。
//
// # 快速上手
//
//	// --- Memory ---
//	p := dlock.NewMemoryProvider()
//	m := p.NewMutex("my-key")
//
//	// --- Redis ---
//	pool := goredisv9.NewPool(redisClient)
//	p := dlock.NewRedisProvider(pool, dlock.WithRedisExpiry(10*time.Second))
//	m := p.NewMutex("my-key")
//
//	// --- etcd ---
//	p, err := dlock.NewEtcdProvider(etcdClient, dlock.WithEtcdTTL(10))
//	m := p.NewMutex("my-key")
//
//	// 通用使用方式
//	if err := m.Lock(ctx); err != nil { ... }
//	defer m.Unlock(ctx)
package dlock

import (
	"context"
	"errors"
)

// ErrLockFailed 获取锁失败（TryLock 未获取到锁时返回）。
var ErrLockFailed = errors.New("dlock: failed to acquire lock")

// Provider 是锁工厂接口，每种后端（Memory / Redis / etcd）实现一次。
type Provider interface {
	// NewMutex 创建一个以 key 为标识的互斥锁。
	// 同一 key 在同一 Provider 下的多个 Mutex 实例之间互斥。
	NewMutex(key string) Mutex
}

// Mutex 是分布式互斥锁的统一接口。
type Mutex interface {
	// Lock 阻塞式获取锁，直到获取成功或 ctx 被取消。
	Lock(ctx context.Context) error

	// TryLock 非阻塞式尝试获取锁，获取失败时返回 ErrLockFailed。
	TryLock(ctx context.Context) error

	// Unlock 释放锁。
	Unlock(ctx context.Context) error
}
