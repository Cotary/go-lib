package nodepool

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Strategy defines how requests are dispatched across nodes.
type Strategy interface {
	Execute(ctx context.Context, nodes []*Node, req *Request, do DoFunc) (*Response, NodeStatus, error)
}

// DoFunc is invoked by a Strategy to execute a request on a specific node.
type DoFunc func(ctx context.Context, node *Node, req *Request) (*Response, NodeStatus, error)

// conservativeStrategy tries nodes one by one in score order. Quota-saving and concurrent-safe.
type conservativeStrategy struct {
	maxRetries     int
	waitOnThrottle bool
}

func newConservativeStrategy(maxRetries int, waitOnThrottle bool) Strategy {
	if maxRetries <= 0 {
		maxRetries = 1
	}
	return &conservativeStrategy{maxRetries: maxRetries, waitOnThrottle: waitOnThrottle}
}

func (s *conservativeStrategy) Execute(ctx context.Context, nodes []*Node, req *Request, do DoFunc) (*Response, NodeStatus, error) {
	if len(nodes) == 0 {
		return nil, NodeStatusFail, fmt.Errorf("nodepool: no available nodes")
	}

	var lastErr error

	for round := 0; round <= s.maxRetries; round++ {
		if ctx.Err() != nil {
			return nil, NodeStatusFail, ctx.Err()
		}

		anyAttempted := false
		var throttledNodes []*Node

		for _, node := range nodes {
			if !node.IsAvailable() {
				continue
			}
			if !node.TryAcquire() {
				throttledNodes = append(throttledNodes, node)
				continue
			}

			anyAttempted = true
			resp, status, err := do(ctx, node, req)
			node.Release()

			switch status {
			case NodeStatusSuccess:
				return resp, status, nil
			case NodeStatusBizError:
				return resp, status, err
			case NodeStatusFail:
				lastErr = err
				continue
			}
		}

		if !anyAttempted && len(throttledNodes) > 0 {
			if s.waitOnThrottle {
				bestNode := throttledNodes[0]
				if err := bestNode.Acquire(ctx); err != nil {
					lastErr = err
					continue
				}
				resp, status, err := do(ctx, bestNode, req)
				bestNode.Release()
				switch status {
				case NodeStatusSuccess:
					return resp, status, nil
				case NodeStatusBizError:
					return resp, status, err
				case NodeStatusFail:
					lastErr = err
				}
			} else if lastErr == nil {
				lastErr = ErrAllThrottled
			}
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("nodepool: all attempts failed")
	}
	return nil, NodeStatusFail, fmt.Errorf("nodepool: exhausted retries: %w", lastErr)
}

// raceStrategy sends requests to multiple nodes concurrently, first success wins.
type raceStrategy struct {
	concurrency    int
	waitOnThrottle bool
}

func newRaceStrategy(concurrency int, waitOnThrottle bool) Strategy {
	return &raceStrategy{concurrency: concurrency, waitOnThrottle: waitOnThrottle}
}

func (s *raceStrategy) Execute(ctx context.Context, nodes []*Node, req *Request, do DoFunc) (*Response, NodeStatus, error) {
	var acquired []*Node
	var throttled []*Node

	for _, n := range nodes {
		if !n.IsAvailable() {
			continue
		}
		if !n.TryAcquire() {
			throttled = append(throttled, n)
			continue
		}
		acquired = append(acquired, n)
	}

	if len(acquired) == 0 {
		if len(throttled) > 0 {
			if s.waitOnThrottle {
				bestNode := throttled[0]
				if err := bestNode.Acquire(ctx); err != nil {
					return nil, NodeStatusFail, err
				}
				resp, status, err := do(ctx, bestNode, req)
				bestNode.Release()
				return resp, status, err
			}
			return nil, NodeStatusFail, ErrAllThrottled
		}
		return nil, NodeStatusFail, fmt.Errorf("nodepool: no available nodes")
	}

	n := len(acquired)
	if s.concurrency > 0 && s.concurrency < n {
		for _, node := range acquired[s.concurrency:] {
			node.Release()
		}
		n = s.concurrency
	}
	candidates := acquired[:n]

	type result struct {
		resp   *Response
		status NodeStatus
		err    error
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan result, n)
	var wg sync.WaitGroup
	wg.Add(n)

	for _, node := range candidates {
		go func(nd *Node) {
			defer wg.Done()
			defer nd.Release()
			resp, status, err := do(ctx, nd, req)
			select {
			case ch <- result{resp, status, err}:
			case <-ctx.Done():
			}
		}(node)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var errs []string
	for r := range ch {
		switch r.status {
		case NodeStatusSuccess:
			cancel()
			return r.resp, r.status, nil
		case NodeStatusBizError:
			cancel()
			return r.resp, r.status, r.err
		case NodeStatusFail:
			if r.err != nil {
				errs = append(errs, r.err.Error())
			}
		}
	}

	return nil, NodeStatusFail, fmt.Errorf("nodepool: all %d race attempts failed: %s", n, strings.Join(errs, "; "))
}
