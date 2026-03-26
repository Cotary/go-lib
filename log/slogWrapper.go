package log

import (
	"context"
	"io"
	"log/slog"
	"runtime"
	"time"
)

type SlogWrapper struct {
	inner      *slog.Logger
	ctx        context.Context
	writer     io.Writer
	fileWriter io.Closer // 仅 root logger 持有，用于关闭 lumberjack
}

func (w *SlogWrapper) log(level slog.Level, msg string, args ...any) {
	if !w.inner.Enabled(w.ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)

	_ = w.inner.Handler().Handle(w.ctx, r)
}
func (w *SlogWrapper) Debug(msg string, args ...any) { w.log(slog.LevelDebug, msg, args...) }
func (w *SlogWrapper) Info(msg string, args ...any)  { w.log(slog.LevelInfo, msg, args...) }
func (w *SlogWrapper) Warn(msg string, args ...any)  { w.log(slog.LevelWarn, msg, args...) }
func (w *SlogWrapper) Error(msg string, args ...any) { w.log(slog.LevelError, msg, args...) }

// Raw 纯记录，不带任何格式化信息（时间戳、级别、调用栈等），不受级别过滤控制
func (w *SlogWrapper) Raw(data string) {
	if w.writer != nil {
		_, _ = w.writer.Write([]byte(data + "\n"))
	}
}

func (w *SlogWrapper) WithField(key string, val any) Logger {
	return &SlogWrapper{inner: w.inner.With(key, val), ctx: w.ctx, writer: w.writer}
}

func (w *SlogWrapper) WithFields(fields map[string]any) Logger {
	if len(fields) == 0 {
		return w
	}
	newFields := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		newFields = append(newFields, k, v)
	}
	return &SlogWrapper{inner: w.inner.With(newFields...), ctx: w.ctx, writer: w.writer}
}

func (w *SlogWrapper) WithContext(ctx context.Context) Logger {
	return &SlogWrapper{inner: w.inner, ctx: ctx, writer: w.writer}
}

// Close 关闭底层文件 writer，应在程序退出前调用以确保日志不丢失。
// 仅对 NewLogger 创建的 root logger 有效，派生 logger 无需调用。
func (w *SlogWrapper) Close() error {
	if w.fileWriter != nil {
		return w.fileWriter.Close()
	}
	return nil
}
