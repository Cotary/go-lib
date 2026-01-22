package log

import (
	"context"
	"log/slog"
	"runtime"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ZapHandler 直接对接 zapcore
type ZapHandler struct {
	core  zapcore.Core
	cfg   *Config
	attrs []slog.Attr // 保存通过 WithAttrs 添加的属性
}

func (h *ZapHandler) Enabled(_ context.Context, l slog.Level) bool {
	// 使用 zapcore 的 Enabled 方法检查级别
	return h.core.Enabled(getZapLevelFromSlog(l))
}

func (h *ZapHandler) Handle(ctx context.Context, r slog.Record) error {
	ent := zapcore.Entry{
		Level:   getZapLevelFromSlog(r.Level),
		Time:    r.Time,
		Message: r.Message,
	}

	if h.cfg.ShowFile && r.PC != 0 {
		f, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
		ent.Caller = zapcore.NewEntryCaller(f.PC, f.File, f.Line, true)
	}

	// 先处理通过 WithAttrs 保存的属性
	fields := make([]zapcore.Field, 0, len(h.attrs)+r.NumAttrs())
	for _, a := range h.attrs {
		// 统一耗时为秒（与 slog 的 ReplaceAttr 保持一致）
		if a.Value.Kind() == slog.KindDuration {
			fields = append(fields, zap.Float64(a.Key, a.Value.Duration().Seconds()))
		} else {
			fields = append(fields, zap.Any(a.Key, a.Value.Any()))
		}
	}

	// 再处理 Record 里的属性
	r.Attrs(func(a slog.Attr) bool {
		// 统一耗时为秒（与 slog 的 ReplaceAttr 保持一致）
		if a.Value.Kind() == slog.KindDuration {
			fields = append(fields, zap.Float64(a.Key, a.Value.Duration().Seconds()))
		} else {
			fields = append(fields, zap.Any(a.Key, a.Value.Any()))
		}
		return true
	})

	return h.core.Write(ent, fields)
}
func (h *ZapHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// 创建新的 handler，包含原有属性和新属性
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &ZapHandler{
		core:  h.core,
		cfg:   h.cfg,
		attrs: newAttrs,
	}
}
func (h *ZapHandler) WithGroup(name string) slog.Handler { return h }

func getZapLevelFromSlog(l slog.Level) zapcore.Level {
	switch l {
	case slog.LevelDebug:
		return zapcore.DebugLevel
	case slog.LevelWarn:
		return zapcore.WarnLevel
	case slog.LevelError:
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
