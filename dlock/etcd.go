package dlock

import (
	"context"
	"errors"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// EtcdOption 配置 EtcdProvider 的选项函数。
type EtcdOption func(*etcdProviderConfig)

type etcdProviderConfig struct {
	ttl       int // Session 的 TTL（秒），也决定锁的租约时长
	keyPrefix string
}

var defaultEtcdConfig = etcdProviderConfig{
	ttl:       10,
	keyPrefix: "/dlock/",
}

// WithEtcdTTL 设置 etcd Session 的 TTL（秒），默认 10。
// Session TTL 决定了当持锁进程崩溃后，锁最长多久自动释放。
func WithEtcdTTL(ttl int) EtcdOption {
	return func(c *etcdProviderConfig) { c.ttl = ttl }
}

// WithEtcdKeyPrefix 设置 etcd key 前缀（默认 "/dlock/"）。
func WithEtcdKeyPrefix(prefix string) EtcdOption {
	return func(c *etcdProviderConfig) { c.keyPrefix = prefix }
}

// EtcdProvider 基于 etcd concurrency 的分布式锁工厂。
type EtcdProvider struct {
	client *clientv3.Client
	cfg    etcdProviderConfig
}

// NewEtcdProvider 创建基于 etcd 的分布式锁工厂。
func NewEtcdProvider(client *clientv3.Client, opts ...EtcdOption) *EtcdProvider {
	cfg := defaultEtcdConfig
	for _, o := range opts {
		o(&cfg)
	}
	return &EtcdProvider{
		client: client,
		cfg:    cfg,
	}
}

// NewMutex 创建一个以 key 为标识的 etcd 分布式锁。
func (p *EtcdProvider) NewMutex(key string) Mutex {
	return &etcdMutex{
		client: p.client,
		pfx:    p.cfg.keyPrefix + key,
		ttl:    p.cfg.ttl,
	}
}

// etcdMutex 每次 Lock 时创建 Session + concurrency.Mutex，Unlock 后释放 Session。
// 这保证了每次获取锁都使用独立的租约，避免 Session 复用带来的生命周期管理问题。
type etcdMutex struct {
	client  *clientv3.Client
	pfx     string
	ttl     int
	session *concurrency.Session
	mu      *concurrency.Mutex
}

func (m *etcdMutex) Lock(ctx context.Context) error {
	session, err := concurrency.NewSession(m.client,
		concurrency.WithTTL(m.ttl),
		concurrency.WithContext(ctx),
	)
	if err != nil {
		return err
	}

	mu := concurrency.NewMutex(session, m.pfx)
	if err := mu.Lock(ctx); err != nil {
		_ = session.Close()
		return err
	}

	m.session = session
	m.mu = mu
	return nil
}

func (m *etcdMutex) TryLock(ctx context.Context) error {
	session, err := concurrency.NewSession(m.client,
		concurrency.WithTTL(m.ttl),
		concurrency.WithContext(ctx),
	)
	if err != nil {
		return err
	}

	mu := concurrency.NewMutex(session, m.pfx)
	if err := mu.TryLock(ctx); err != nil {
		_ = session.Close()
		if errors.Is(err, concurrency.ErrLocked) {
			return ErrLockFailed
		}
		return err
	}

	m.session = session
	m.mu = mu
	return nil
}

func (m *etcdMutex) Unlock(ctx context.Context) error {
	if m.mu == nil {
		return nil
	}
	err := m.mu.Unlock(ctx)
	if m.session != nil {
		_ = m.session.Close()
	}
	m.mu = nil
	m.session = nil
	return err
}
