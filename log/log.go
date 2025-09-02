package log

import (
	"context"
	"github.com/Cotary/go-lib/common/defined"
)

var ISO8601TimeLayout = "2006-01-02T15:04:05.000Z07:00"

// Config 日志配置
// MaxAge和MaxBackups满足一个即可被清除
type Config struct {
	Level      string `mapstructure:"level"`      // 日志级别
	Path       string `mapstructure:"path"`       // 日志文件路径
	FileSuffix string `mapstructure:"fileSuffix"` // 日志文件后缀
	MaxAge     int64  `mapstructure:"maxAge"`     // 日志文件最大保存时间（24小时）: 0默认30天,-1不限制
	MaxBackups int64  `mapstructure:"maxBackups"` // 日志文件最大数量（备份数），默认不限制
	MaxSize    int64  `mapstructure:"maxSize"`    // 日志文件大小 MB,lumberjack默认100MB,不能无限增长
	FileName   string `mapstructure:"fileName"`   // 日志文件名
	Compress   bool   `mapstructure:"compress"`   // 是否压缩,默认不压缩
}

func handleConfig(config *Config) {
	if config.Level == "" {
		config.Level = "debug"
	}
	if config.Path == "" {
		config.Path = "./logs/"
	}
	if config.FileSuffix == "" {
		config.FileSuffix = ".log"
	}
	if config.FileName == "" {
		config.FileName = "runtime"
	}
	if config.MaxAge == 0 {
		config.MaxAge = 30
	} else if config.MaxAge < 0 {
		config.MaxAge = 0
	}

	if config.MaxSize == 0 {
		config.MaxSize = 100
	}
}

var globalLogger Logger

func SetGlobalLogger(logger Logger) {
	globalLogger = logger
}

func WithContext(ctx context.Context) Logger {
	if globalLogger == nil {
		globalLogger = NewZapLogger(&Config{}).WithContext(ctx)
	}

	fields := make(map[string]interface{})
	if val := ctx.Value(defined.RequestID); val != nil {
		fields[defined.RequestID] = val
	}
	if val := ctx.Value(defined.RequestURI); val != nil {
		fields[defined.RequestURI] = val
	}
	if val := ctx.Value(defined.RequestBodyJson); val != nil {
		fields[defined.RequestBodyJson] = val
	}

	return globalLogger.WithContext(ctx).WithFields(fields)
}
