package handler

import (
	"context"
	"log/slog"
)

// RedactedValue 是敏感字段的替换值。
const RedactedValue = "***"

// redactHandler 把指定 key 的值改写成 ***，分组内的同名字段同样处理。
type redactHandler struct {
	next slog.Handler
	keys map[string]struct{}
}

func (h *redactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *redactHandler) Handle(ctx context.Context, r slog.Record) error {
	if len(h.keys) > 0 {
		r = redactRecord(r, h.keys)
	}
	return h.next.Handle(ctx, r)
}

func (h *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &redactHandler{next: h.next.WithAttrs(redactAttrs(attrs, h.keys)), keys: h.keys}
}

func (h *redactHandler) WithGroup(name string) slog.Handler {
	return &redactHandler{next: h.next.WithGroup(name), keys: h.keys}
}

func redactAttrs(attrs []slog.Attr, keys map[string]struct{}) []slog.Attr {
	if len(keys) == 0 {
		return attrs
	}
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i], _ = redactAttr(a, keys)
	}
	return out
}

func redactRecord(r slog.Record, keys map[string]struct{}) slog.Record {
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	changed := false
	r.Attrs(func(a slog.Attr) bool {
		na, c := redactAttr(a, keys)
		changed = changed || c
		nr.AddAttrs(na)
		return true
	})
	if !changed {
		return r
	}
	return nr
}

func redactAttr(a slog.Attr, keys map[string]struct{}) (slog.Attr, bool) {
	if a.Value.Kind() == slog.KindGroup {
		group := a.Value.Group()
		out := make([]slog.Attr, 0, len(group))
		changed := false
		for _, ga := range group {
			na, c := redactAttr(ga, keys)
			changed = changed || c
			out = append(out, na)
		}
		if !changed {
			return a, false
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}, true
	}
	if _, ok := keys[a.Key]; ok {
		return slog.String(a.Key, RedactedValue), true
	}
	return a, false
}
