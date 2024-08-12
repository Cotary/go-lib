package log

import (
	"context"
	"github.com/Cotary/go-lib/common/defined"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
)

type Logs struct {
	Level         string `yaml:"level"`
	Path          string `yaml:"path"`
	FileSuffix    string `yaml:"fileSuffix"`
	MaxAgeHour    int64  `yaml:"maxAgeHour"`
	RotationCount uint   `yaml:"rotationCount"`
}

var DefaultLogger *logrus.Logger

func InitLogger(logs *Logs) {
	DefaultLogger = logrus.New()
	level, _ := logrus.ParseLevel(logs.Level)
	DefaultLogger.SetLevel(level)
	DefaultLogger.SetReportCaller(false)
	DefaultLogger.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02 15:04:05", DisableTimestamp: false, PrettyPrint: false})
	writer, _ := rotatelogs.New(logs.Path+"%Y%m%d"+logs.FileSuffix,
		rotatelogs.WithRotationTime(24*time.Hour),
		rotatelogs.WithRotationCount(logs.RotationCount),
		rotatelogs.WithRotationTime(time.Hour*time.Duration(logs.MaxAgeHour)),
	)
	DefaultLogger.AddHook(lfshook.NewHook(writer, &logrus.JSONFormatter{TimestampFormat: "2006-01-02 15:04:05"}))
}
func WithContext(ctx context.Context) *logrus.Entry {
	if ctx.Value(defined.RequestID) != nil {
		return DefaultLogger.WithContext(ctx).WithField(defined.RequestID, ctx.Value(defined.RequestID))
	}
	return DefaultLogger.WithContext(ctx)
}
