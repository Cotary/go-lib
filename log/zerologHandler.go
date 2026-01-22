package log

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"

	"github.com/rs/zerolog"
)

// ZerologHandler 自定义适配器
type ZerologHandler struct {
	logger zerolog.Logger
	cfg    *Config
	attrs  []slog.Attr // 保存通过 WithAttrs 添加的属性
}

func (h *ZerologHandler) Enabled(_ context.Context, l slog.Level) bool {
	return h.logger.GetLevel() <= getZerologLevel(l.String())
}

func (h *ZerologHandler) Handle(ctx context.Context, r slog.Record) error {
	// 1. 根据级别开启事件
	var event *zerolog.Event
	switch r.Level {
	case slog.LevelDebug:
		event = h.logger.Debug()
	case slog.LevelInfo:
		event = h.logger.Info()
	case slog.LevelWarn:
		event = h.logger.Warn()
	case slog.LevelError:
		event = h.logger.Error()
	default:
		event = h.logger.Info()
	}

	if event == nil {
		return nil
	}

	// 2. 注入时间 (统一 key 为 time)
	event.Time(zerolog.TimestampFieldName, r.Time)

	// 3. 处理调用栈 (行号)
	if h.cfg.ShowFile && r.PC != 0 {
		f, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
		event.Str("caller", fmt.Sprintf("%s:%d", filepath.Base(f.File), f.Line))
	}

	// 先处理通过 WithAttrs 保存的属性
	for _, a := range h.attrs {
		// 统一耗时为秒（与 slog 的 ReplaceAttr 保持一致）
		if a.Value.Kind() == slog.KindDuration {
			event.Float64(a.Key, a.Value.Duration().Seconds())
		} else {
			event.Any(a.Key, a.Value.Any())
		}
	}

	// 再处理 Record 里的属性
	r.Attrs(func(a slog.Attr) bool {
		// 统一耗时为秒（与 slog 的 ReplaceAttr 保持一致）
		if a.Value.Kind() == slog.KindDuration {
			event.Float64(a.Key, a.Value.Duration().Seconds())
		} else {
			event.Any(a.Key, a.Value.Any())
		}
		return true
	})

	event.Msg(r.Message)
	return nil
}

func (h *ZerologHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// 创建新的 handler，包含原有属性和新属性
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &ZerologHandler{
		logger: h.logger,
		cfg:    h.cfg,
		attrs:  newAttrs,
	}
}
func (h *ZerologHandler) WithGroup(name string) slog.Handler { return h }
