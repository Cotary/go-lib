package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cotary/go-lib/common/defined"
	"github.com/natefinch/lumberjack"
	"github.com/rs/zerolog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	DriverZap     string = "zap"
	DriverZerolog string = "zerolog"
	DriverSlog    string = "slog"
)

const (
	FormatJSON = "json"
	FormatText = "text"
)

type Config struct {
	Driver     string `mapstructure:"driver" yaml:"driver"`         // 驱动: zap, zerolog, slog
	Level      string `mapstructure:"level" yaml:"level"`           // 级别: debug, info, warn, error
	Path       string `mapstructure:"path" yaml:"path"`             // 路径
	FileName   string `mapstructure:"fileName" yaml:"fileName"`     // 文件名
	FileSuffix string `mapstructure:"fileSuffix" yaml:"fileSuffix"` // 后缀
	MaxAge     int64  `mapstructure:"maxAge" yaml:"maxAge"`         // 最大保留天数
	MaxBackups int64  `mapstructure:"maxBackups" yaml:"maxBackups"` // 最大备份数
	MaxSize    int64  `mapstructure:"maxSize" yaml:"maxSize"`       // 单文件大小MB
	Compress   bool   `mapstructure:"compress" yaml:"compress"`     // 是否压缩
	ShowFile   bool   `mapstructure:"showFile" yaml:"showFile"`     // 是否显示代码行号
	Format     string `mapstructure:"format" yaml:"format"`         // 格式: json, text
}

var globalLogger Logger

func SetGlobalLogger(l Logger) {
	globalLogger = l
}

func NewLogger(cfg *Config) *SlogWrapper {
	handleConfig(cfg)

	writer := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.Path, cfg.FileName+cfg.FileSuffix),
		MaxSize:    int(cfg.MaxSize),
		MaxBackups: int(cfg.MaxBackups),
		MaxAge:     int(cfg.MaxAge),
		Compress:   cfg.Compress,
		LocalTime:  true,
	}
	output := io.MultiWriter(os.Stdout, writer)

	var handler slog.Handler

	switch cfg.Driver {
	case DriverZap:
		encCfg := zap.NewProductionEncoderConfig()
		encCfg.TimeKey = "time"
		encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		encCfg.EncodeDuration = zapcore.SecondsDurationEncoder
		encCfg.EncodeCaller = zapcore.ShortCallerEncoder

		var encoder zapcore.Encoder
		if cfg.Format == FormatJSON {
			encoder = zapcore.NewJSONEncoder(encCfg)
		} else {
			encoder = zapcore.NewConsoleEncoder(encCfg)
		}

		core := zapcore.NewCore(encoder, zapcore.AddSync(output), zap.NewAtomicLevelAt(getZapLevel(cfg.Level)))
		handler = &ZapHandler{core: core, cfg: cfg}

	case DriverZerolog:
		zerolog.TimestampFieldName = "time"
		zerolog.TimeFieldFormat = "2006-01-02T15:04:05.000Z0700"

		var zOut io.Writer = output
		if cfg.Format == FormatText {
			zOut = zerolog.ConsoleWriter{Out: output, TimeFormat: "15:04:05"}
		}

		zLogger := zerolog.New(zOut).Level(getZerologLevel(cfg.Level))
		handler = &ZerologHandler{logger: zLogger, cfg: cfg}

	default:
		opts := &slog.HandlerOptions{
			Level:     getSlogLevel(cfg.Level),
			AddSource: cfg.ShowFile,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					a.Key = "time"
				}
				// 统一耗时为秒
				if a.Value.Kind() == slog.KindDuration {
					return slog.Float64(a.Key, a.Value.Duration().Seconds())
				}
				return a
			},
		}
		if cfg.Format == FormatJSON {
			handler = slog.NewJSONHandler(output, opts)
		} else {
			handler = slog.NewTextHandler(output, opts)
		}
	}

	return &SlogWrapper{
		inner:  slog.New(handler),
		ctx:    context.Background(),
		writer: output,
	}
}

// handleConfig 处理默认配置
func handleConfig(config *Config) {
	if config.Driver == "" {
		config.Driver = DriverZap
	}
	if config.Level == "" {
		config.Level = "debug"
	}
	if config.Path == "" {
		config.Path = "./logs/"
	}
	if config.FileSuffix == "" {
		config.FileSuffix = ".logger"
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
		config.MaxSize = 10
	}
	if config.Format == "" {
		config.Format = FormatJSON
	}
}

func getSlogLevel(l string) slog.Level {
	switch strings.ToLower(l) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func getZapLevel(l string) zapcore.Level {
	switch strings.ToLower(l) {
	case "debug":
		return zap.DebugLevel
	case "warn":
		return zap.WarnLevel
	case "error":
		return zap.ErrorLevel
	default:
		return zap.InfoLevel
	}
}

func getZerologLevel(l string) zerolog.Level {
	switch strings.ToLower(l) {
	case "debug":
		return zerolog.DebugLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

func WithContext(ctx context.Context) Logger {
	if globalLogger == nil {
		globalLogger = NewLogger(&Config{})
	}
	fields := make(map[string]any)
	if val := ctx.Value(defined.RequestID); val != nil {
		fields[defined.RequestID] = val
	}
	return globalLogger.WithContext(ctx).WithFields(fields)
}
