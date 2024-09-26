package log

import (
	"context"
	"github.com/Cotary/go-lib/common/defined"
)

type Config struct {
	Level         string `yaml:"level"`         // 日志级别
	Path          string `yaml:"path"`          // 日志文件路径
	FileSuffix    string `yaml:"fileSuffix"`    // 日志文件后缀
	MaxAgeHour    int64  `yaml:"maxAgeHour"`    // 日志文件最大保存时间（小时）
	RotationTime  int64  `yaml:"rotationTime"`  // 日志文件轮转时间
	RotationCount int64  `yaml:"rotationCount"` // 日志文件最大数量
	RotationSize  int64  `yaml:"rotationSize"`  // 日志文件大小 MB
	FileName      string `yaml:"fileName"`      // 日志文件名
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
		config.FileName = "%Y%m%d"
	}

	if config.MaxAgeHour == 0 {
		config.MaxAgeHour = 24 * 30
	} else if config.MaxAgeHour < 0 {
		config.MaxAgeHour = 0
	}
	if config.RotationTime == 0 {
		config.RotationTime = 24
	} else if config.RotationTime < 0 {
		config.RotationTime = 0
	}

	if config.MaxAgeHour > 0 {
		config.RotationCount = 0
	} else if config.RotationCount < 0 {
		config.RotationCount = 0
	}

	if config.RotationSize == 0 {
		config.RotationSize = 100
	} else if config.RotationSize < 0 {
		config.RotationSize = 0
	}
}

var GlobalLogger Logger

func WithContext(ctx context.Context) Logger {
	if GlobalLogger == nil {
		GlobalLogger = NewZapLogger(&Config{}).WithContext(ctx)
	}
	return GlobalLogger.WithContext(ctx).WithFields(map[string]interface{}{
		defined.RequestID:       ctx.Value(defined.RequestID),
		defined.RequestURI:      ctx.Value(defined.RequestURI),
		defined.RequestBodyJson: ctx.Value(defined.RequestBodyJson),
	})
}
