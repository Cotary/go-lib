package nodepool

import (
	"context"
	"errors"
	"time"
)

// ErrAllThrottled is returned when all nodes are rate-limited and WaitOnThrottle is false.
var ErrAllThrottled = errors.New("nodepool: all nodes throttled")

// NodeStatus indicates how a response affects node health.
type NodeStatus int

const (
	// NodeStatusSuccess - request succeeded, node is healthy.
	NodeStatusSuccess NodeStatus = iota
	// NodeStatusFail - node-level failure (rate limited, timeout, internal error).
	// Triggers retry on another node and counts against node health.
	NodeStatusFail
	// NodeStatusBizError - business-level error (invalid params, data not found).
	// Does NOT retry, does NOT affect node health scoring.
	NodeStatusBizError
)

// Request is a protocol-agnostic request envelope.
type Request struct {
	Data any
}

// Response is a protocol-agnostic response envelope.
type Response struct {
	Data    any
	Latency time.Duration
}

// Transport abstracts the actual request execution over any protocol.
type Transport interface {
	Execute(ctx context.Context, endpoint string, req *Request) (*Response, error)
}

// ResponseClassifier determines the outcome of a response for node health tracking.
// The endpoint parameter identifies which node produced this response,
// enabling per-node classification logic (e.g. different rate limit formats).
type ResponseClassifier interface {
	Classify(ctx context.Context, endpoint string, resp *Response, err error) NodeStatus
}

// DefaultClassifier treats any non-nil error as NodeStatusFail, otherwise NodeStatusSuccess.
type DefaultClassifier struct{}

func (DefaultClassifier) Classify(_ context.Context, _ string, _ *Response, err error) NodeStatus {
	if err != nil {
		return NodeStatusFail
	}
	return NodeStatusSuccess
}

// RateLimitConfig configures per-node rate limiting.
type RateLimitConfig struct {
	MaxConcurrent int     // Max concurrent in-flight requests, 0 = unlimited.
	PerSecond     float64 // Max requests per second (token bucket rate), 0 = unlimited.
	PerMinute     float64 // Max requests per minute (token bucket rate), 0 = unlimited.
	Burst         int     // Token bucket burst capacity. Defaults to max(1, int(PerSecond)) if 0.
}

// NodeConfig holds the configuration for a single node/endpoint.
type NodeConfig struct {
	Endpoint   string
	Weight     int
	Meta       map[string]any
	Classifier ResponseClassifier // Optional per-node classifier. If nil, the pool-level classifier is used.
	RateLimit  *RateLimitConfig   // Optional per-node rate limiting. If nil, no rate limiting is applied.
}

// NodeStats holds runtime statistics for a node (read-only snapshot).
type NodeStats struct {
	Endpoint        string
	TotalRequests   int64
	SuccessCount    int64
	FailCount       int64
	ConsecutiveFail int
	ConsecutiveSucc int
	AvgLatency      time.Duration
	CircuitOpen     bool
}
