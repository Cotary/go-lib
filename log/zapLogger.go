package log

import (
	"context"
	"fmt"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"time"
)

type ZapLogger struct {
	Logger  *zap.Logger
	Context context.Context
}

func NewZapLogger(config *Config) *ZapLogger {
	handleConfig(config)
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
	writeSyncer := zapcore.AddSync(writer)
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	//encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder //把日志级别转换成大写字母
	encoderConfig.EncodeDuration = zapcore.SecondsDurationEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	var level = new(zapcore.Level)
	err = level.UnmarshalText([]byte(config.Level))
	if err != nil {
		panic(err)
	}
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		writeSyncer,
		level,
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	return &ZapLogger{Logger: logger, Context: context.Background()}
}

func (z *ZapLogger) Debug(msg string, fields ...map[string]interface{}) {
	z.Logger.Debug(msg, convertZapFields(fields)...)
}

func (z *ZapLogger) Info(msg string, fields ...map[string]interface{}) {
	z.Logger.Info(msg, convertZapFields(fields)...)
}

func (z *ZapLogger) Warn(msg string, fields ...map[string]interface{}) {
	z.Logger.Warn(msg, convertZapFields(fields)...)
}

func (z *ZapLogger) Error(msg string, fields ...map[string]interface{}) {
	z.Logger.Error(msg, convertZapFields(fields)...)
}

func (z *ZapLogger) Fatal(msg string, fields ...map[string]interface{}) {
	z.Logger.Fatal(msg, convertZapFields(fields)...)
}

func (z *ZapLogger) WithContext(ctx context.Context) Logger {
	return &ZapLogger{Logger: z.Logger, Context: ctx}
}

func (z *ZapLogger) WithField(key string, value interface{}) Logger {
	return &ZapLogger{Logger: z.Logger.With(zap.Any(key, value)), Context: z.Context}
}

func (z *ZapLogger) WithFields(fields map[string]interface{}) Logger {
	return &ZapLogger{Logger: z.Logger.With(convertZapFields([]map[string]interface{}{fields})...), Context: z.Context}
}

func convertZapFields(fields []map[string]interface{}) []zap.Field {
	var zapFields []zap.Field
	for _, fieldMap := range fields {
		for key, value := range fieldMap {
			zapFields = append(zapFields, zap.Any(key, value))
		}
	}
	return zapFields
}
