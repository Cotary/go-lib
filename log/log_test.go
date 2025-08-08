package log_test

import (
	"context"
	"github.com/Cotary/go-lib/log"
	"testing"
)

func TestZapLoggerOutput(t *testing.T) {
	cfg := &log.Config{
		Level:      "debug",
		Path:       "./test-logs/",
		FileSuffix: ".log",
		MaxAge:     1,
		MaxSize:    10,
		MaxBackups: 5,
		FileName:   "zap-test-log",
		Compress:   false,
	}

	logger := log.NewZapLogger(cfg)
	log.SetGlobalLogger(logger)

	ctx := context.WithValue(context.Background(), "request_id", "123456")
	globalLogger := log.WithContext(ctx)

	globalLogger.Debug("This is a debug message", map[string]interface{}{"module": "test", "step": "debug"})
	globalLogger.Info("This is an info message", map[string]interface{}{"module": "test", "step": "info"})
	globalLogger.Warn("This is a warning", map[string]interface{}{"module": "test", "step": "warn"})
	globalLogger.Error("This is an error", map[string]interface{}{"module": "test", "step": "error"})
	// logger.Fatal("This is a fatal error", map[string]interface{}{"module": "test", "step": "fatal"}) // 会退出程序，不建议在测试中启用
}
func TestLogrusLoggerOutput(t *testing.T) {
	cfg := &log.Config{
		Level:      "debug",
		Path:       "./test-logs/",
		FileSuffix: ".log",
		MaxAge:     1,
		MaxSize:    10,
		MaxBackups: 5,
		FileName:   "logrus-test-log",
		Compress:   false,
	}

	logger := log.NewLogrusLogger(cfg)
	log.SetGlobalLogger(logger)

	ctx := context.WithValue(context.Background(), "request_id", "abcdef")
	globalLogger := log.WithContext(ctx)

	globalLogger.Debug("This is a debug message", map[string]interface{}{"module": "test", "step": "debug"})
	globalLogger.Info("This is an info message", map[string]interface{}{"module": "test", "step": "info"})
	globalLogger.Warn("This is a warning", map[string]interface{}{"module": "test", "step": "warn"})
	globalLogger.Error("This is an error", map[string]interface{}{"module": "test", "step": "error"})
	// logger.Fatal("Logrus fatal message", map[string]interface{}{"module": "logrus-test", "step": "fatal"}) // 注意 fatal 会退出程序
}

func TestZapLoggerRolling(t *testing.T) {
	cfg := &log.Config{
		Level:      "debug",
		Path:       "./test-logs/",
		FileSuffix: ".log",
		MaxAge:     1, // 仅保留 1 天
		MaxSize:    1, // 设置较小值以触发滚动，单位 MB
		MaxBackups: 3, // 最多保留 3 个备份
		FileName:   "",
		Compress:   false, // 启用压缩功能
	}

	logger := log.NewZapLogger(cfg)
	log.SetGlobalLogger(logger)

	ctx := context.Background()
	globalLogger := log.WithContext(ctx)

	for i := 0; i < 5000; i++ {
		globalLogger.Info("Rolling log entry", map[string]interface{}{
			"line":     i,
			"category": "rotation-test",
			"payload":  string(make([]byte, 1024)), // 每条日志约 1KB
		})
	}

	t.Log("Completed Zap rolling test")
}
func TestLogrusLoggerRolling(t *testing.T) {
	cfg := &log.Config{
		Level:      "debug",
		Path:       "./test-logs/",
		FileSuffix: ".log",
		MaxAge:     1,
		MaxSize:    1,
		MaxBackups: 3,
		FileName:   "logrus-rolling",
		Compress:   false,
	}

	logger := log.NewLogrusLogger(cfg)
	log.SetGlobalLogger(logger)

	ctx := context.Background()
	globalLogger := log.WithContext(ctx)

	for i := 0; i < 5000; i++ {
		globalLogger.Info("Rolling log entry", map[string]interface{}{
			"line":     i,
			"category": "rotation-test",
			"payload":  string(make([]byte, 1024)),
		})
	}

	t.Log("Completed Logrus rolling test")
}
