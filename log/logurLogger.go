package log

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

type LogrusLogger struct {
	Entry *logrus.Entry
}

func NewLogrusLogger(cfg *Config) *LogrusLogger {
	handleConfig(cfg)

	writer := &lumberjack.Logger{
		Filename:   fmt.Sprintf("%s%s%s", cfg.Path, cfg.FileName, cfg.FileSuffix),
		MaxSize:    int(cfg.MaxSize),
		MaxBackups: int(cfg.MaxBackups),
		MaxAge:     int(cfg.MaxAge),
		Compress:   cfg.Compress,
		LocalTime:  true,
	}

	logger := logrus.New()
	formatter := &logrus.JSONFormatter{
		TimestampFormat: ISO8601TimeLayout,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			skip := 8
			pc := make([]uintptr, 1)
			n := runtime.Callers(skip, pc)
			if n == 0 {
				return "unknown()", "unknown:0"
			}
			frame, _ := runtime.CallersFrames(pc).Next()
			filename := filepath.Base(frame.File)
			return fmt.Sprintf("%s()", frame.Function), fmt.Sprintf("%s:%d", filename, frame.Line)
		},
	}

	logger.SetFormatter(formatter)
	logger.SetReportCaller(true) // 启用 caller 信息
	logger.SetOutput(io.MultiWriter(os.Stdout, writer))

	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	return &LogrusLogger{Entry: logrus.NewEntry(logger)}
}

func (l *LogrusLogger) Debug(msg string, fields ...map[string]interface{}) {
	l.Entry.WithFields(toLogrusFields(fields)).Debug(msg)
}
func (l *LogrusLogger) Info(msg string, fields ...map[string]interface{}) {
	l.Entry.WithFields(toLogrusFields(fields)).Info(msg)
}
func (l *LogrusLogger) Warn(msg string, fields ...map[string]interface{}) {
	l.Entry.WithFields(toLogrusFields(fields)).Warn(msg)
}
func (l *LogrusLogger) Error(msg string, fields ...map[string]interface{}) {
	l.Entry.WithFields(toLogrusFields(fields)).Error(msg)
}
func (l *LogrusLogger) Fatal(msg string, fields ...map[string]interface{}) {
	l.Entry.WithFields(toLogrusFields(fields)).Fatal(msg)
}

func (l *LogrusLogger) WithContext(ctx context.Context) Logger {
	return &LogrusLogger{Entry: l.Entry.WithContext(ctx)}
}

func (l *LogrusLogger) WithField(key string, value interface{}) Logger {
	return &LogrusLogger{Entry: l.Entry.WithField(key, value)}
}

func (l *LogrusLogger) WithFields(fields map[string]interface{}) Logger {
	return &LogrusLogger{Entry: l.Entry.WithFields(fields)}
}

func toLogrusFields(fs []map[string]interface{}) logrus.Fields {
	fields := logrus.Fields{}
	for _, m := range fs {
		for k, v := range m {
			fields[k] = v
		}
	}
	return fields
}
