package log

import "context"

type Logger interface {
	Debug(msg string, fields ...map[string]interface{})
	Info(msg string, fields ...map[string]interface{})
	Warn(msg string, fields ...map[string]interface{})
	Error(msg string, fields ...map[string]interface{})
	Fatal(msg string, fields ...map[string]interface{})
	WithContext(ctx context.Context) Logger
	WithField(key string, value interface{}) Logger
	WithFields(map[string]interface{}) Logger
}
