package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Cotary/go-lib/common/defined"
	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	DriverZap  string = "zap"
	DriverSlog string = "slog"
)

const (
	FormatJSON = "json"
	FormatText = "text"
)

type Config struct {
	Driver     string `mapstructure:"driver" yaml:"driver"`
	Level      string `mapstructure:"level" yaml:"level"`
	Path       string `mapstructure:"path" yaml:"path"`
	FileName   string `mapstructure:"fileName" yaml:"fileName"`
	FileSuffix string `mapstructure:"fileSuffix" yaml:"fileSuffix"`
	MaxAge     int64  `mapstructure:"maxAge" yaml:"maxAge"`
	MaxBackups int64  `mapstructure:"maxBackups" yaml:"maxBackups"`
	MaxSize    int64  `mapstructure:"maxSize" yaml:"maxSize"`
	Compress   bool   `mapstructure:"compress" yaml:"compress"`
	ShowFile   bool   `mapstructure:"showFile" yaml:"showFile"`
	Format     string `mapstructure:"format" yaml:"format"`
}

var (
	globalLogger Logger
	mu           sync.RWMutex
	defaultOnce  sync.Once
)

func SetGlobalLogger(l Logger) {
	mu.Lock()
	defer mu.Unlock()
	globalLogger = l
}

func NewLogger(cfg *Config) *SlogWrapper {
	c := *cfg
	handleConfig(&c)

	writer := &lumberjack.Logger{
		Filename:   filepath.Join(c.Path, c.FileName+c.FileSuffix),
		MaxSize:    int(c.MaxSize),
		MaxBackups: int(c.MaxBackups),
		MaxAge:     int(c.MaxAge),
		Compress:   c.Compress,
		LocalTime:  true,
	}
	output := io.MultiWriter(os.Stdout, writer)

	var handler slog.Handler

	switch c.Driver {
	case DriverZap:
		encCfg := zap.NewProductionEncoderConfig()
		encCfg.TimeKey = "time"
		encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		encCfg.EncodeDuration = zapcore.SecondsDurationEncoder
		encCfg.EncodeCaller = zapcore.ShortCallerEncoder

		var encoder zapcore.Encoder
		if c.Format == FormatJSON {
			encoder = zapcore.NewJSONEncoder(encCfg)
		} else {
			encoder = zapcore.NewConsoleEncoder(encCfg)
		}

		core := zapcore.NewCore(encoder, zapcore.AddSync(output), zap.NewAtomicLevelAt(getZapLevel(c.Level)))
		handler = &ZapHandler{core: core, cfg: &c}

	default:
		opts := &slog.HandlerOptions{
			Level:     getSlogLevel(c.Level),
			AddSource: c.ShowFile,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					a.Key = "time"
				}
				if a.Value.Kind() == slog.KindDuration {
					return slog.Float64(a.Key, a.Value.Duration().Seconds())
				}
				return a
			},
		}
		if c.Format == FormatJSON {
			handler = slog.NewJSONHandler(output, opts)
		} else {
			handler = slog.NewTextHandler(output, opts)
		}
	}

	return &SlogWrapper{
		inner:      slog.New(handler),
		ctx:        context.Background(),
		writer:     output,
		fileWriter: writer,
	}
}

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

func WithContext(ctx context.Context) Logger {
	mu.RLock()
	l := globalLogger
	mu.RUnlock()

	if l == nil {
		defaultOnce.Do(func() {
			mu.Lock()
			defer mu.Unlock()
			if globalLogger == nil {
				globalLogger = NewLogger(&Config{})
			}
		})
		mu.RLock()
		l = globalLogger
		mu.RUnlock()
	}

	fields := make(map[string]any)
	if val := ctx.Value(defined.RequestID); val != nil {
		fields[defined.RequestID] = val
	}
	return l.WithContext(ctx).WithFields(fields)
}
