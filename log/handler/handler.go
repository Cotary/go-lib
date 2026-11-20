// Package handler 是日志的 Handler 链实现：采样、ctx 字段注入、脱敏、告警。
//
// 由 log 包装配，也可以单独使用：
//
//	h := handler.New(os.Stdout, handler.Config{Level: slog.LevelInfo})
//	logger := slog.New(h)
package handler

import (
	"io"
	"log/slog"
)

// Config 是 Handler 链的配置。
type Config struct {
	Level         slog.Level
	AddSource     bool
	Console       bool
	Alerter       Alerter
	RedactKeys    map[string]struct{}
	ContextFields []ContextField
	SampleEvery   uint64
	SampleBelow   slog.Level
	Fields        []slog.Attr
}

// New 按固定顺序装配 Handler：采样 -> ctx 字段 -> 脱敏 -> 告警 -> 输出。
// 告警在脱敏之后，投递出去的字段也是脱敏后的。
func New(w io.Writer, c Config) slog.Handler {
	opts := &slog.HandlerOptions{
		Level:     c.Level,
		AddSource: c.AddSource,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// 沿用原 log 包 slog 分支的配置：Duration 统一成秒。
			if a.Value.Kind() == slog.KindDuration {
				return slog.Float64(a.Key, a.Value.Duration().Seconds())
			}
			return a
		},
	}

	var h slog.Handler
	if c.Console {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}

	h = &alertHandler{next: h, alerter: c.Alerter}
	h = &redactHandler{next: h, keys: c.RedactKeys}
	h = newContextHandler(h, c.ContextFields)
	h = newSampleHandler(h, c.SampleEvery, c.SampleBelow)

	if len(c.Fields) > 0 {
		h = h.WithAttrs(c.Fields)
	}
	return h
}

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
