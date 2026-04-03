package nodepool

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Pool manages a set of nodes and dispatches requests according to the configured strategy.
type Pool struct {
	mu         sync.RWMutex
	opts       Options
	transport  Transport
	classifier ResponseClassifier
	strategy   Strategy
	nodes      []*Node
	nodeMap    map[string]*Node
	cancel     context.CancelFunc
}

// New creates a new Pool. transport is required; classifier is optional (defaults to DefaultClassifier).
func New(transport Transport, classifier ResponseClassifier, configs []NodeConfig, opts ...Option) (*Pool, error) {
	if transport == nil {
		return nil, fmt.Errorf("nodepool: transport is required")
	}
	if len(configs) == 0 {
		return nil, fmt.Errorf("nodepool: at least one node config is required")
	}

	o := defaultOptions()
	for _, fn := range opts {
		fn(&o)
	}

	if classifier == nil {
		classifier = DefaultClassifier{}
	}

	var st Strategy
	switch o.Strategy {
	case StrategyConservative:
		st = newConservativeStrategy(o.MaxRetries, o.WaitOnThrottle)
	case StrategyRace:
		st = newRaceStrategy(o.RaceConcurrency, o.WaitOnThrottle)
	default:
		st = newConservativeStrategy(o.MaxRetries, o.WaitOnThrottle)
	}

	p := &Pool{
		opts:       o,
		transport:  transport,
		classifier: classifier,
		strategy:   st,
		nodeMap:    make(map[string]*Node),
	}

	for _, cfg := range configs {
		n := newNode(cfg, o)
		p.nodes = append(p.nodes, n)
		p.nodeMap[cfg.Endpoint] = n
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.backgroundLoop(ctx)

	return p, nil
}

// Close stops the background health watcher.
func (p *Pool) Close() {
	if p.cancel != nil {
		p.cancel()
	}
}

// Do executes a single request through the pool.
func (p *Pool) Do(ctx context.Context, req *Request) (*Response, error) {
	resp, _, err := p.doWithStatus(ctx, req)
	return resp, err
}

func (p *Pool) doWithStatus(ctx context.Context, req *Request) (*Response, NodeStatus, error) {
	p.mu.RLock()
	sorted := make([]*Node, len(p.nodes))
	copy(sorted, p.nodes)
	p.mu.RUnlock()

	return p.strategy.Execute(ctx, sorted, req, p.doOnNode)
}

// DoMulti executes multiple requests in parallel, each independently dispatched.
func (p *Pool) DoMulti(ctx context.Context, reqs []*Request) ([]*Response, []error) {
	responses := make([]*Response, len(reqs))
	errs := make([]error, len(reqs))

	var wg sync.WaitGroup
	wg.Add(len(reqs))
	for i, req := range reqs {
		go func(idx int, r *Request) {
			defer wg.Done()
			resp, err := p.Do(ctx, r)
			responses[idx] = resp
			errs[idx] = err
		}(i, req)
	}
	wg.Wait()
	return responses, errs
}

// AddNode adds a node dynamically. If the endpoint already exists, it updates the config.
func (p *Pool) AddNode(cfg NodeConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.nodeMap[cfg.Endpoint]; ok {
		existing.updateConfig(cfg)
		return
	}
	n := newNode(cfg, p.opts)
	p.nodes = append(p.nodes, n)
	p.nodeMap[cfg.Endpoint] = n
}

// RemoveNode removes a node by its endpoint address.
func (p *Pool) RemoveNode(endpoint string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.nodeMap[endpoint]; !ok {
		return
	}
	delete(p.nodeMap, endpoint)
	filtered := make([]*Node, 0, len(p.nodes)-1)
	for _, n := range p.nodes {
		if n.Endpoint() != endpoint {
			filtered = append(filtered, n)
		}
	}
	p.nodes = filtered
}

// UpdateNodes syncs the node list: adds new ones, updates existing, removes stale.
func (p *Pool) UpdateNodes(configs []NodeConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()

	newMap := make(map[string]NodeConfig, len(configs))
	for _, cfg := range configs {
		newMap[cfg.Endpoint] = cfg
	}

	for _, cfg := range configs {
		if existing, ok := p.nodeMap[cfg.Endpoint]; ok {
			existing.updateConfig(cfg)
		} else {
			n := newNode(cfg, p.opts)
			p.nodes = append(p.nodes, n)
			p.nodeMap[cfg.Endpoint] = n
		}
	}

	filtered := make([]*Node, 0, len(configs))
	for _, n := range p.nodes {
		if _, ok := newMap[n.Endpoint()]; ok {
			filtered = append(filtered, n)
		} else {
			delete(p.nodeMap, n.Endpoint())
		}
	}
	p.nodes = filtered
}

// NodeStats returns a snapshot of all node statistics.
func (p *Pool) NodeStats() []NodeStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	stats := make([]NodeStats, len(p.nodes))
	for i, n := range p.nodes {
		stats[i] = n.Stats()
	}
	return stats
}

// NodeCount returns the current number of nodes.
func (p *Pool) NodeCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.nodes)
}

func (p *Pool) doOnNode(ctx context.Context, node *Node, req *Request) (*Response, NodeStatus, error) {
	if !node.tryTransitionToHalfOpen() {
		return nil, NodeStatusFail, fmt.Errorf("nodepool: node %s circuit open", node.Endpoint())
	}

	start := time.Now()
	resp, err := p.transport.Execute(ctx, node.Endpoint(), req)
	latency := time.Since(start)

	if resp != nil {
		resp.Latency = latency
	}

	classifier := node.Config().Classifier
	if classifier == nil {
		classifier = p.classifier
	}
	status := classifier.Classify(ctx, node.Endpoint(), resp, err)

	switch status {
	case NodeStatusSuccess:
		node.RecordSuccess(latency)
	case NodeStatusFail:
		node.RecordFailure(latency)
	case NodeStatusBizError:
		node.RecordBizError()
	}

	return resp, status, err
}

func (p *Pool) backgroundLoop(ctx context.Context) {
	sortTicker := time.NewTicker(p.opts.SortPeriod)
	defer sortTicker.Stop()

	for {
		select {
		case <-sortTicker.C:
			p.resortNodes()
		case <-ctx.Done():
			return
		}
	}
}

func (p *Pool) resortNodes() {
	p.mu.Lock()
	defer p.mu.Unlock()
	sort.SliceStable(p.nodes, func(i, j int) bool {
		return p.nodes[i].Score() > p.nodes[j].Score()
	})
}

// DoUntilComplete executes all requests, automatically retrying node-level failures
// until every request succeeds or the context is cancelled.
// BizErrors are NOT retried (business-level errors won't be fixed by switching nodes).
// Callers MUST use a context with timeout/deadline to prevent indefinite blocking.
func (p *Pool) DoUntilComplete(ctx context.Context, reqs []*Request) ([]*Response, []error) {
	responses := make([]*Response, len(reqs))
	errs := make([]error, len(reqs))

	pending := make([]int, len(reqs))
	for i := range reqs {
		pending[i] = i
	}

	for round := 0; len(pending) > 0; round++ {
		if ctx.Err() != nil {
			for _, idx := range pending {
				errs[idx] = ctx.Err()
			}
			break
		}

		if round > 0 {
			backoff := time.Duration(round) * 500 * time.Millisecond
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				for _, idx := range pending {
					errs[idx] = ctx.Err()
				}
				return responses, errs
			}
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		nextPending := make([]int, 0)

		wg.Add(len(pending))
		for _, idx := range pending {
			go func(i int) {
				defer wg.Done()
				resp, status, err := p.doWithStatus(ctx, reqs[i])
				switch status {
				case NodeStatusSuccess:
					responses[i] = resp
				case NodeStatusBizError:
					responses[i] = resp
					errs[i] = err
				default:
					mu.Lock()
					nextPending = append(nextPending, i)
					mu.Unlock()
				}
			}(idx)
		}
		wg.Wait()
		pending = nextPending
	}

	return responses, errs
}
