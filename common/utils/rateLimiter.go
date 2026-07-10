package utils

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimiter 通用速率限流器
// 基于令牌桶算法（golang.org/x/time/rate），支持按 key 分组限流
type RateLimiter struct {
	global   *rate.Limiter
	limiters sync.Map // key -> *rate.Limiter
	limit    rate.Limit
	burst    int
	mu       sync.RWMutex
}

// NewRateLimiter 创建限流器
//   - perSecond: 每秒允许的请求数
//   - burst:     突发容量（允许瞬间通过的最大请求数）
func NewRateLimiter(perSecond float64, burst int) *RateLimiter {
	limit := rate.Limit(perSecond)
	return &RateLimiter{
		global: rate.NewLimiter(limit, burst),
		limit:  limit,
		burst:  burst,
	}
}

// Wait 阻塞等待直到获取到令牌，或 ctx 被取消
// 用于全局限流（所有调用共享同一个桶）
func (r *RateLimiter) Wait(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.global.Wait(ctx)
}

// WaitKey 按 key 分组限流，不同 key 各自独立计数
// 适用于"每个商户独立限流"、"每个 API 独立限流"等场景
func (r *RateLimiter) WaitKey(ctx context.Context, key string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	limiter := r.getKeyLimiter(key)
	return limiter.Wait(ctx)
}

// Allow 非阻塞判断：当前是否允许通过（不等待）
func (r *RateLimiter) Allow() bool {
	return r.global.Allow()
}

// AllowKey 按 key 非阻塞判断
func (r *RateLimiter) AllowKey(key string) bool {
	return r.getKeyLimiter(key).Allow()
}

// SetRate 动态调整全局速率（运行时生效）
// 注意：已按 key 创建的 limiter 不会自动更新
func (r *RateLimiter) SetRate(perSecond float64, burst int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limit = rate.Limit(perSecond)
	r.burst = burst
	r.global.SetLimit(r.limit)
	r.global.SetBurst(burst)
}

func (r *RateLimiter) getKeyLimiter(key string) *rate.Limiter {
	if v, ok := r.limiters.Load(key); ok {
		return v.(*rate.Limiter)
	}
	r.mu.RLock()
	limit := r.limit
	burst := r.burst
	r.mu.RUnlock()
	limiter := rate.NewLimiter(limit, burst)
	actual, _ := r.limiters.LoadOrStore(key, limiter)
	return actual.(*rate.Limiter)
}
