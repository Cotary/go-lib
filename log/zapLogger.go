package log

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
)

type ZapLogger struct {
	Logger  *zap.Logger
	Context context.Context
}

func NewZapLogger(cfg *Config) *ZapLogger {
	handleConfig(cfg)

	writer := &lumberjack.Logger{
		Filename:   fmt.Sprintf("%s%s%s", cfg.Path, cfg.FileName, cfg.FileSuffix),
		MaxSize:    int(cfg.MaxSize),
		MaxBackups: int(cfg.MaxBackups),
		MaxAge:     int(cfg.MaxAge),
		Compress:   cfg.Compress,
		LocalTime:  true,
	}

	writeSyncer := zapcore.AddSync(writer)
	consoleSyncer := zapcore.AddSync(os.Stdout)

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	//encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder //把日志级别转换成大写字母
	encoderConfig.EncodeDuration = zapcore.SecondsDurationEncoder //将日志中的 Duration 类型字段转换为以秒为单位的数字输出。
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder       //显示日志触发位置的简洁形式：main.go:42

	encoder := zapcore.NewJSONEncoder(encoderConfig)

	var lvl = new(zapcore.Level)
	if err := lvl.UnmarshalText([]byte(cfg.Level)); err != nil {
		*lvl = zapcore.InfoLevel
	}

	core := zapcore.NewTee(
		zapcore.NewCore(encoder, writeSyncer, lvl),
		zapcore.NewCore(encoder, consoleSyncer, lvl),
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(2))
	return &ZapLogger{Logger: logger, Context: context.Background()}
}

func (z *ZapLogger) log(method func(string, ...zap.Field), msg string, fields ...map[string]interface{}) {
	method(msg, convertZapFields(fields)...)
}

func (z *ZapLogger) Debug(msg string, fields ...map[string]interface{}) {
	z.log(z.Logger.Debug, msg, fields...)
}
func (z *ZapLogger) Info(msg string, fields ...map[string]interface{}) {
	z.log(z.Logger.Info, msg, fields...)
}
func (z *ZapLogger) Warn(msg string, fields ...map[string]interface{}) {
	z.log(z.Logger.Warn, msg, fields...)
}
func (z *ZapLogger) Error(msg string, fields ...map[string]interface{}) {
	z.log(z.Logger.Error, msg, fields...)
}
func (z *ZapLogger) Fatal(msg string, fields ...map[string]interface{}) {
	z.log(z.Logger.Fatal, msg, fields...)
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
	for _, m := range fields {
		for k, v := range m {
			zapFields = append(zapFields, zap.Any(k, v))
		}
	}
	return zapFields
}
