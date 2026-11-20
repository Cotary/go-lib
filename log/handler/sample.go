package handler

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// sampleHandler 对低于 below 级别的记录按 1/every 抽样；
// 告警记录（带 AlertKey）永远不抽，避免把告警抽没。
type sampleHandler struct {
	next  slog.Handler
	every uint64
	below slog.Level
	n     *atomic.Uint64
}

func newSampleHandler(next slog.Handler, every uint64, below slog.Level) *sampleHandler {
	return &sampleHandler{next: next, every: every, below: below, n: new(atomic.Uint64)}
}

func (h *sampleHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *sampleHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.every > 1 && r.Level < h.below && !recordHasKey(r, AlertKey) {
		if h.n.Add(1)%h.every != 0 {
			return nil
		}
	}
	return h.next.Handle(ctx, r)
}

func (h *sampleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &sampleHandler{next: h.next.WithAttrs(attrs), every: h.every, below: h.below, n: h.n}
}

func (h *sampleHandler) WithGroup(name string) slog.Handler {
	return &sampleHandler{next: h.next.WithGroup(name), every: h.every, below: h.below, n: h.n}
}
