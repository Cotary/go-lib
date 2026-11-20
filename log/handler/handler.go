// Package handler 提供可组合的 slog Handler。
//
// 每个构造函数只接收必要参数并返回 Middleware，next 由 Chain 按数组顺序注入：
//
//	h := Chain(base,
//	        NewSample(100, slog.LevelWarn),
//	        NewContext(ContextField{Key: requestID, Name: "request_id"}),
//	        NewAlert(alerter))
//
// 不配置就不组合，没有隐式的默认能力。
package handler

import (
	"log/slog"
)

// ---------- 工具 ----------

func recordHasKey(r slog.Record, key string) bool {
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = true
			return false
		}
		return true
	})
	return found
}

func withoutAttr(r slog.Record, key string) slog.Record {
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key != key {
			nr.AddAttrs(a)
		}
		return true
	})
	return nr
}

// Middleware 包装一个 Handler，返回新的 Handler。
type Middleware func(next slog.Handler) slog.Handler

// Chain 按数组顺序组合中间件：mws[0] 最外层，它的 next 是 mws[1]，依次到 base。
//
//	Chain(base, NewContext(...), NewRedact(...), NewAlert(...))
//	// 等价于 NewContext(NewRedact(NewAlert(base)))
func Chain(base slog.Handler, mws ...Middleware) slog.Handler {
	h := base
	for i := len(mws) - 1; i >= 0; i-- {
		if mws[i] == nil {
			continue
		}
		h = mws[i](h)
	}
	return h
}
