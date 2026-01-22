package log

import "context"

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Raw(data string) // 纯记录，不带任何格式化信息
	WithField(key string, val any) Logger
	WithFields(fields map[string]any) Logger
	WithContext(ctx context.Context) Logger
}
