package nodepool

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
)

// circuitState represents the circuit breaker state.
type circuitState int

const (
	circuitClosed   circuitState = iota // normal operation
	circuitOpen                         // failing, reject requests
	circuitHalfOpen                     // testing recovery
)

// Node wraps a single endpoint with health tracking, circuit breaker, and rate limiting.
type Node struct {
	mu       sync.RWMutex
	config   NodeConfig
	opts     Options
	state    circuitState
	openedAt time.Time

	totalRequests   int64
	successCount    int64
	failCount       int64
	consecutiveFail int
	consecutiveSucc int
	avgLatency      time.Duration
	score           float64

	concSem       *semaphore.Weighted
	rateLimiter   *rate.Limiter
	minuteLimiter *rate.Limiter
}

func newNode(cfg NodeConfig, opts Options) *Node {
	w := cfg.Weight
	if w <= 0 {
		w = 1
	}
	n := &Node{
		config: cfg,
		opts:   opts,
		state:  circuitClosed,
		score:  float64(w),
	}
	n.initThrottler()
	return n
}

func (n *Node) initThrottler() {
	rl := n.config.RateLimit
	if rl == nil {
		return
	}
	if rl.MaxConcurrent > 0 {
		n.concSem = semaphore.NewWeighted(int64(rl.MaxConcurrent))
	}
	if rl.PerSecond > 0 {
		burst := rl.Burst
		if burst <= 0 {
			burst = int(rl.PerSecond)
			if burst <= 0 {
				burst = 1
			}
		}
		n.rateLimiter = rate.NewLimiter(rate.Limit(rl.PerSecond), burst)
	}
	if rl.PerMinute > 0 {
		perSec := rl.PerMinute / 60.0
		burst := rl.Burst
		if burst <= 0 {
			burst = int(perSec)
			if burst <= 0 {
				burst = 1
			}
		}
		n.minuteLimiter = rate.NewLimiter(rate.Limit(perSec), burst)
	}
}

// TryAcquire attempts to acquire rate limit tokens non-blockingly.
// Returns true if the node can accept a request right now.
// The caller MUST call Release() after the request completes if TryAcquire returns true.
func (n *Node) TryAcquire() bool {
	if n.concSem == nil && n.rateLimiter == nil && n.minuteLimiter == nil {
		return true
	}
	if n.concSem != nil && !n.concSem.TryAcquire(1) {
		return false
	}
	if n.rateLimiter != nil && !n.rateLimiter.Allow() {
		if n.concSem != nil {
			n.concSem.Release(1)
		}
		return false
	}
	if n.minuteLimiter != nil && !n.minuteLimiter.Allow() {
		if n.concSem != nil {
			n.concSem.Release(1)
		}
		return false
	}
	return true
}

// Acquire blocks until rate limit tokens are available or ctx is cancelled.
// The caller MUST call Release() after the request completes if Acquire returns nil.
func (n *Node) Acquire(ctx context.Context) error {
	if n.concSem == nil && n.rateLimiter == nil && n.minuteLimiter == nil {
		return nil
	}
	if n.concSem != nil {
		if err := n.concSem.Acquire(ctx, 1); err != nil {
			return err
		}
	}
	if n.rateLimiter != nil {
		if err := n.rateLimiter.Wait(ctx); err != nil {
			if n.concSem != nil {
				n.concSem.Release(1)
			}
			return err
		}
	}
	if n.minuteLimiter != nil {
		if err := n.minuteLimiter.Wait(ctx); err != nil {
			if n.concSem != nil {
				n.concSem.Release(1)
			}
			return err
		}
	}
	return nil
}

// Release releases the concurrency semaphore slot acquired by TryAcquire or Acquire.
func (n *Node) Release() {
	if n.concSem != nil {
		n.concSem.Release(1)
	}
}

// Endpoint returns the node's endpoint address.
func (n *Node) Endpoint() string {
	return n.config.Endpoint
}

// Config returns the node's configuration.
func (n *Node) Config() NodeConfig {
	return n.config
}

// IsAvailable returns true if the node can accept requests.
func (n *Node) IsAvailable() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	switch n.state {
	case circuitClosed:
		return true
	case circuitHalfOpen:
		return true
	case circuitOpen:
		if time.Since(n.openedAt) >= n.opts.CircuitOpenTime {
			return true // will transition to half-open on next use
		}
		return false
	}
	return false
}

// Score returns the current score (higher is better).
func (n *Node) Score() float64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.score
}

// Stats returns a snapshot of the node's statistics.
func (n *Node) Stats() NodeStats {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return NodeStats{
		Endpoint:        n.config.Endpoint,
		TotalRequests:   n.totalRequests,
		SuccessCount:    n.successCount,
		FailCount:       n.failCount,
		ConsecutiveFail: n.consecutiveFail,
		ConsecutiveSucc: n.consecutiveSucc,
		AvgLatency:      n.avgLatency,
		CircuitOpen:     n.state == circuitOpen,
	}
}

// RecordSuccess records a successful request and updates health metrics.
func (n *Node) RecordSuccess(latency time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.totalRequests++
	n.successCount++
	n.consecutiveSucc++
	n.consecutiveFail = 0
	n.updateLatency(latency)

	switch n.state {
	case circuitHalfOpen:
		if n.consecutiveSucc >= n.opts.SuccessThreshold {
			n.state = circuitClosed
		}
	case circuitOpen:
		n.state = circuitHalfOpen
		n.consecutiveSucc = 1
	}

	n.recalcScore()
}

// RecordFailure records a node-level failure and updates health metrics.
func (n *Node) RecordFailure(latency time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.totalRequests++
	n.failCount++
	n.consecutiveFail++
	n.consecutiveSucc = 0
	n.updateLatency(latency)

	if n.consecutiveFail >= n.opts.FailureThreshold {
		n.state = circuitOpen
		n.openedAt = time.Now()
	}

	n.recalcScore()
}

// RecordBizError records a business error (no impact on node health).
func (n *Node) RecordBizError() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.totalRequests++
}

func (n *Node) updateLatency(d time.Duration) {
	if n.avgLatency == 0 {
		n.avgLatency = d
	} else {
		alpha := n.opts.EWMAAlpha
		n.avgLatency = time.Duration(float64(n.avgLatency)*(1-alpha) + float64(d)*alpha)
	}
}

func (n *Node) recalcScore() {
	w := float64(n.config.Weight)
	if w <= 0 {
		w = 1
	}

	var successRate float64
	if n.totalRequests > 0 {
		successRate = float64(n.successCount) / float64(n.totalRequests)
	}

	var latencyPenalty float64
	if n.avgLatency > 0 {
		latencyPenalty = float64(time.Second) / float64(n.avgLatency)
		if latencyPenalty > 10 {
			latencyPenalty = 10
		}
	}

	failPenalty := float64(n.consecutiveFail) * 0.5

	n.score = w*successRate + latencyPenalty - failPenalty
	if n.state == circuitOpen {
		n.score = -1
	}
}

// tryTransitionToHalfOpen attempts to move from open to half-open. Returns true if the node can be tried.
func (n *Node) tryTransitionToHalfOpen() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.state == circuitOpen && time.Since(n.openedAt) >= n.opts.CircuitOpenTime {
		n.state = circuitHalfOpen
		n.consecutiveSucc = 0
		return true
	}
	return n.state != circuitOpen
}

func (n *Node) updateConfig(cfg NodeConfig) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.config = cfg
}
