package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/Cotary/go-lib/log/handler"
)

// 公开类型转发到 log/handler，调用方仍然只 import log。
type (
	Alert        = handler.Alert
	Alerter      = handler.Alerter
	ContextField = handler.ContextField
)

// AlertKey 是"这条日志要告警"的标记字段名。
const AlertKey = handler.AlertKey

// Factory 构造一个 slog.Handler，writer 和级别都由调用方在闭包里捕获。
//
// 例如：
//
//	log.New(log.JSON(os.Stdout, slog.LevelInfo))
type Factory func() slog.Handler

// New 组合 0..N 个 Factory，返回标准库 *slog.Logger。
//
//   - 不传：原样输出到 os.Stdout，级别 Info；
//   - 一个：直接使用；
//   - 多个：用 slog.NewMultiHandler 组合。
func New(factories ...Factory) (*slog.Logger, error) {
	var handlers []slog.Handler
	for _, f := range factories {
		if f == nil {
			continue
		}
		h := f()
		if h == nil {
			return nil, fmt.Errorf("log: handler factory returned nil")
		}
		handlers = append(handlers, h)
	}
	if len(handlers) == 0 {
		handlers = append(handlers, Raw(os.Stdout, slog.LevelInfo)())
	}

	var h slog.Handler = handlers[0]
	if len(handlers) > 1 {
		h = slog.NewMultiHandler(handlers...)
	}

	return slog.New(h), nil
}

// replaceAttr 统一处理内置格式的属性：Duration 转秒、剥离告警标记。
func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == handler.AlertKey {
		return slog.Attr{}
	}
	if a.Value.Kind() == slog.KindDuration {
		return slog.Float64(a.Key, a.Value.Duration().Seconds())
	}
	return a
}

// JSON 返回写往 w 的 JSON Handler 工厂（内置 Duration→秒）。
func JSON(w io.Writer, level slog.Level) Factory {
	return func() slog.Handler {
		return slog.NewJSONHandler(w, &slog.HandlerOptions{
			Level:       level,
			AddSource:   true,
			ReplaceAttr: replaceAttr,
		})
	}
}

// Text 返回写往 w 的 Text Handler 工厂（内置 Duration→秒）。
func Text(w io.Writer, level slog.Level) Factory {
	return func() slog.Handler {
		return slog.NewTextHandler(w, &slog.HandlerOptions{
			Level:       level,
			AddSource:   true,
			ReplaceAttr: replaceAttr,
		})
	}
}

// Raw 返回写往 w 的原样输出工厂：只写 msg + 换行。
func Raw(w io.Writer, level slog.Level) Factory {
	return func() slog.Handler {
		return handler.NewRaw(w, level)
	}
}

// ParseLevel 把级别名解析成 slog.Level；未知或为空时回退到 info。
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug", "trace", "verbose":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "err", "fatal":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithFields 把 map 展开成 slog 参数（按 key 排序，输出稳定）。
func WithFields(fields map[string]any) []any {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]any, 0, len(fields)*2)
	for _, k := range keys {
		out = append(out, k, fields[k])
	}
	return out
}
