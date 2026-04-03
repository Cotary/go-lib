package nodepool

import "time"

// StrategyType defines the execution mode.
type StrategyType int

const (
	// StrategyConservative tries nodes one by one (quota-saving, concurrent-safe).
	StrategyConservative StrategyType = iota
	// StrategyRace sends to multiple nodes simultaneously, first success wins.
	StrategyRace
)

// Options configures the Pool behavior.
type Options struct {
	Strategy          StrategyType
	MaxRetries        int
	RaceConcurrency   int
	FailureThreshold  int
	SuccessThreshold  int
	CircuitOpenTime   time.Duration
	HealthCheckPeriod time.Duration
	SortPeriod        time.Duration
	EWMAAlpha         float64
	WaitOnThrottle    bool // If true, block when all nodes are throttled until one becomes available.
}

func defaultOptions() Options {
	return Options{
		Strategy:          StrategyConservative,
		MaxRetries:        3,
		RaceConcurrency:   0,
		FailureThreshold:  5,
		SuccessThreshold:  3,
		CircuitOpenTime:   10 * time.Second,
		HealthCheckPeriod: 30 * time.Second,
		SortPeriod:        3 * time.Second,
		EWMAAlpha:         0.3,
	}
}

// Option is a functional option for configuring Pool.
type Option func(*Options)

// WithStrategy sets the execution strategy.
func WithStrategy(s StrategyType) Option {
	return func(o *Options) { o.Strategy = s }
}

// WithMaxRetries sets the maximum retry attempts for conservative mode.
func WithMaxRetries(n int) Option {
	return func(o *Options) {
		if n > 0 {
			o.MaxRetries = n
		}
	}
}

// WithRaceConcurrency sets how many nodes to race in race mode.
// 0 means all available nodes.
func WithRaceConcurrency(n int) Option {
	return func(o *Options) { o.RaceConcurrency = n }
}

// WithFailureThreshold sets consecutive failures before circuit opens.
func WithFailureThreshold(n int) Option {
	return func(o *Options) {
		if n > 0 {
			o.FailureThreshold = n
		}
	}
}

// WithSuccessThreshold sets consecutive successes to close a half-open circuit.
func WithSuccessThreshold(n int) Option {
	return func(o *Options) {
		if n > 0 {
			o.SuccessThreshold = n
		}
	}
}

// WithCircuitOpenTime sets how long a circuit stays open before half-open.
func WithCircuitOpenTime(d time.Duration) Option {
	return func(o *Options) {
		if d > 0 {
			o.CircuitOpenTime = d
		}
	}
}

// WithHealthCheckPeriod sets the interval for health status logging.
func WithHealthCheckPeriod(d time.Duration) Option {
	return func(o *Options) {
		if d > 0 {
			o.HealthCheckPeriod = d
		}
	}
}

// WithSortPeriod sets how often nodes are re-scored and sorted.
func WithSortPeriod(d time.Duration) Option {
	return func(o *Options) {
		if d > 0 {
			o.SortPeriod = d
		}
	}
}

// WithEWMAAlpha sets the EWMA smoothing factor for latency tracking (0..1).
func WithEWMAAlpha(alpha float64) Option {
	return func(o *Options) {
		if alpha > 0 && alpha <= 1 {
			o.EWMAAlpha = alpha
		}
	}
}

// WithWaitOnThrottle sets whether to block when all nodes are rate-limited.
// If true, the pool waits for the best-scored node to become available.
// If false (default), returns ErrAllThrottled immediately.
func WithWaitOnThrottle(wait bool) Option {
	return func(o *Options) { o.WaitOnThrottle = wait }
}
