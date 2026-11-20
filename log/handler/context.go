package handler

import (
	"context"
	"log/slog"
)

// ContextField 描述一个要从 ctx 提取并写入日志的字段。
//
// Key 可以是 string，也可以是自定义类型的 ctx key（推荐后者，避免冲突）；
// Name 是日志字段名。Name 为空时：string key 用 key 本身，其他类型用 fmt.Sprint(key)。
type ContextField struct {
	Key  any
	Name string
}

// contextHandler 从 ctx 取一组字段注入每条日志。
//
// 取不到、值为空或已经通过 With 显式传过的字段会被跳过。
type contextHandler struct {
	next   slog.Handler
	fields []ContextField
	set    []bool // 与 fields 一一对应：With 阶段是否已经显式带了该字段
}

func newContextHandler(next slog.Handler, fields []ContextField) *contextHandler {
	return &contextHandler{next: next, fields: fields, set: make([]bool, len(fields))}
}

func (h *contextHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil && len(h.fields) > 0 {
		var out slog.Record
		added := false
		for i, f := range h.fields {
			if h.set[i] || recordHasKey(r, f.Name) {
				continue
			}
			v, ok := ctx.Value(f.Key).(string)
			if !ok || v == "" {
				continue
			}
			if !added {
				out = r.Clone()
				added = true
			}
			out.AddAttrs(slog.String(f.Name, v))
		}
		if added {
			r = out
		}
	}
	return h.next.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	set := append([]bool(nil), h.set...)
	for _, a := range attrs {
		for i, f := range h.fields {
			if a.Key == f.Name {
				set[i] = true
			}
		}
	}
	return &contextHandler{next: h.next.WithAttrs(attrs), fields: h.fields, set: set}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{next: h.next.WithGroup(name), fields: h.fields, set: h.set}
}
