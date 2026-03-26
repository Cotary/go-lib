package dlock

import (
	"context"
	"errors"
	"time"

	"github.com/go-redsync/redsync/v4"
	redsyncredis "github.com/go-redsync/redsync/v4/redis"
)

// RedisOption 配置 RedisProvider 的选项函数。
type RedisOption func(*redisProviderConfig)

type redisProviderConfig struct {
	expiry     time.Duration
	tries      int
	retryDelay time.Duration
	keyPrefix  string
}

var defaultRedisConfig = redisProviderConfig{
	expiry:     8 * time.Second,
	tries:      32,
	retryDelay: 500 * time.Millisecond,
	keyPrefix:  "dlock:",
}

// WithRedisExpiry 设置锁的过期时间（默认 8s）。
func WithRedisExpiry(d time.Duration) RedisOption {
	return func(c *redisProviderConfig) { c.expiry = d }
}

// WithRedisTries 设置获取锁的最大重试次数（默认 32）。
func WithRedisTries(n int) RedisOption {
	return func(c *redisProviderConfig) { c.tries = n }
}

// WithRedisRetryDelay 设置重试间隔（默认 500ms）。
func WithRedisRetryDelay(d time.Duration) RedisOption {
	return func(c *redisProviderConfig) { c.retryDelay = d }
}

// WithRedisKeyPrefix 设置 Redis key 前缀（默认 "dlock:"）。
func WithRedisKeyPrefix(prefix string) RedisOption {
	return func(c *redisProviderConfig) { c.keyPrefix = prefix }
}

// RedisProvider 基于 redsync 的分布式锁工厂。
type RedisProvider struct {
	rs  *redsync.Redsync
	cfg redisProviderConfig
}

// NewRedisProvider 创建基于 Redis 的分布式锁工厂。
// pool 参数可通过 github.com/go-redsync/redsync/v4/redis/goredis/v9.NewPool 创建。
func NewRedisProvider(pool redsyncredis.Pool, opts ...RedisOption) *RedisProvider {
	cfg := defaultRedisConfig
	for _, o := range opts {
		o(&cfg)
	}
	return &RedisProvider{
		rs:  redsync.New(pool),
		cfg: cfg,
	}
}

// NewMutex 创建一个以 key 为标识的 Redis 分布式锁。
func (p *RedisProvider) NewMutex(key string) Mutex {
	return &redisMutex{
		mu: p.rs.NewMutex(
			p.cfg.keyPrefix+key,
			redsync.WithExpiry(p.cfg.expiry),
			redsync.WithTries(p.cfg.tries),
			redsync.WithRetryDelay(p.cfg.retryDelay),
		),
	}
}

// redisMutex 封装 redsync.Mutex，实现 Mutex 接口。
type redisMutex struct {
	mu *redsync.Mutex
}

func (m *redisMutex) Lock(ctx context.Context) error {
	if err := m.mu.LockContext(ctx); err != nil {
		if errors.Is(err, redsync.ErrFailed) {
			return ErrLockFailed
		}
		return err
	}
	return nil
}

func (m *redisMutex) TryLock(ctx context.Context) error {
	if err := m.mu.TryLockContext(ctx); err != nil {
		if errors.Is(err, redsync.ErrFailed) {
			return ErrLockFailed
		}
		return err
	}
	return nil
}

func (m *redisMutex) Unlock(ctx context.Context) error {
	_, err := m.mu.UnlockContext(ctx)
	if err != nil && errors.Is(err, redsync.ErrLockAlreadyExpired) {
		return nil
	}
	return err
}
