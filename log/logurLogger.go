package log

import (
	"context"
	"fmt"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	"time"
)

type LogrusLogger struct {
	Entry *logrus.Entry
}

func NewLogrusLogger(config *Config) *LogrusLogger {
	handleConfig(config)
	logger := logrus.New()
	timeFormat := "2006-01-02T15:04:05.000Z0700"
	logger.SetFormatter(&logrus.JSONFormatter{TimestampFormat: timeFormat})

	writer, err := rotatelogs.New(
		fmt.Sprintf("%s%s%s", config.Path, config.FileName, config.FileSuffix),
		rotatelogs.WithRotationTime(time.Duration(config.RotationTime)*time.Hour),
		rotatelogs.WithRotationCount(uint(config.RotationCount)),
		rotatelogs.WithMaxAge(time.Duration(config.MaxAgeHour)*time.Hour),
		rotatelogs.WithRotationSize(config.RotationSize*1024*1024),
	)
	if err != nil {
		panic(err)
	}

	logger.AddHook(lfshook.NewHook(writer, &logrus.JSONFormatter{TimestampFormat: timeFormat}))
	logger.SetReportCaller(false)
	logger.SetLevel(parseLogrusLevel(config.Level))
	return &LogrusLogger{Entry: logrus.NewEntry(logger)}
}

func (l *LogrusLogger) Debug(msg string, fields ...map[string]interface{}) {
	l.Entry.WithFields(convertLogrusFields(fields)).Debug(msg)
}

func (l *LogrusLogger) Info(msg string, fields ...map[string]interface{}) {
	l.Entry.WithFields(convertLogrusFields(fields)).Info(msg)
}

func (l *LogrusLogger) Warn(msg string, fields ...map[string]interface{}) {
	l.Entry.WithFields(convertLogrusFields(fields)).Warn(msg)
}

func (l *LogrusLogger) Error(msg string, fields ...map[string]interface{}) {
	l.Entry.WithFields(convertLogrusFields(fields)).Error(msg)
}

func (l *LogrusLogger) Fatal(msg string, fields ...map[string]interface{}) {
	l.Entry.WithFields(convertLogrusFields(fields)).Fatal(msg)
}

func (l *LogrusLogger) WithContext(ctx context.Context) Logger {
	entry := l.Entry.WithContext(ctx)
	return &LogrusLogger{Entry: entry}
}

func (l *LogrusLogger) WithField(key string, value interface{}) Logger {
	return &LogrusLogger{Entry: l.Entry.WithField(key, value)}
}

func (l *LogrusLogger) WithFields(fields map[string]interface{}) Logger {
	return &LogrusLogger{Entry: l.Entry.WithFields(fields)}
}

func convertLogrusFields(fields []map[string]interface{}) logrus.Fields {
	fieldMap := logrus.Fields{}
	for _, field := range fields {
		for key, value := range field {
			fieldMap[key] = value
		}
	}
	return fieldMap
}

// 解析 Logrus 日志级别
func parseLogrusLevel(level string) logrus.Level {
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return logrus.InfoLevel
	}
	return lvl
}
