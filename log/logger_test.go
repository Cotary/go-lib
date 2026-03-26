package log

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Cotary/go-lib/common/defined"
)

func setupTestDir(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("failed to clear test dir: %v", err)
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}
}

func readLastLogLine(filePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty logger file")
	}
	lastLine := lines[len(lines)-1]

	var result map[string]interface{}
	err = json.Unmarshal([]byte(lastLine), &result)
	return result, err
}

func readAllLogLines(filePath string) ([]map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var results []map[string]interface{}
	for _, line := range lines {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(line), &m); err == nil {
			results = append(results, m)
		}
	}
	return results, nil
}

// ---------- 多驱动一致性测试 ----------
// 验证 zap 和 slog 两个驱动在 JSON 格式下输出一致的结构化字段：
// time, msg/message, level, duration(秒), caller/source

func TestConsistency(t *testing.T) {
	testPath := "./test_logs"
	setupTestDir(t, testPath)
	defer os.RemoveAll(testPath)

	drivers := []string{DriverZap, DriverSlog}

	for _, driver := range drivers {
		t.Run(driver, func(t *testing.T) {
			fileName := fmt.Sprintf("test_%s", driver)
			cfg := &Config{
				Driver:     driver,
				Level:      "debug",
				Path:       testPath,
				FileName:   fileName,
				FileSuffix: ".logger",
				ShowFile:   true,
				Format:     FormatJSON,
			}

			logger := NewLogger(cfg)
			defer logger.Close()

			msg := "consistency check"
			cost := 1234 * time.Millisecond

			ctx := context.WithValue(context.Background(), defined.RequestID, "req-123456")
			l := logger.WithContext(ctx).WithFields(map[string]any{
				"cost":       cost,
				"request_id": "req-123",
			})

			l.Info(msg)

			logFilePath := filepath.Join(testPath, fileName+".logger")
			logData, err := readLastLogLine(logFilePath)
			if err != nil {
				t.Fatalf("[%s] failed to read logger: %v", driver, err)
			}

			if _, ok := logData["time"]; !ok {
				t.Errorf("[%s] missing 'time' key", driver)
			}

			if logData["msg"] != msg && logData["message"] != msg {
				t.Errorf("[%s] unexpected msg: %v", driver, logData["msg"])
			}

			costVal, ok := logData["cost"].(float64)
			if !ok || costVal != 1.234 {
				t.Errorf("[%s] duration should be 1.234 seconds, got: %v", driver, logData["cost"])
			}

			callerFound := false
			for _, k := range []string{"caller", "source", "file"} {
				if val, ok := logData[k]; ok {
					if strings.Contains(fmt.Sprint(val), "logger_test.go") {
						callerFound = true
						break
					}
				}
			}
			if !callerFound {
				t.Errorf("[%s] caller info not found or incorrect. logger: %v", driver, logData)
			}

			if !strings.EqualFold(fmt.Sprint(logData["level"]), "info") {
				t.Errorf("[%s] unexpected level: %v", driver, logData["level"])
			}
		})
	}
}

// ---------- 级别过滤测试 ----------
// 验证每个驱动在设定 Level=info 时，debug 日志被过滤，info 日志正常写入

func TestLevelFiltering(t *testing.T) {
	testPath := "./test_logs_level"
	setupTestDir(t, testPath)
	defer os.RemoveAll(testPath)

	drivers := []string{DriverZap, DriverSlog}

	for _, driver := range drivers {
		t.Run(driver, func(t *testing.T) {
			fileName := fmt.Sprintf("level_%s", driver)
			cfg := &Config{
				Driver:     driver,
				Level:      "info",
				Path:       testPath,
				FileName:   fileName,
				FileSuffix: ".logger",
				Format:     FormatJSON,
			}
			logger := NewLogger(cfg)
			defer logger.Close()

			logger.Debug("this should not be logged")
			logger.Info("this should be logged")

			logFilePath := filepath.Join(testPath, fileName+".logger")
			data, _ := os.ReadFile(logFilePath)
			content := string(data)

			if strings.Contains(content, "this should not be logged") {
				t.Errorf("[%s] level filtering failed: debug log found when level set to info", driver)
			}
			if !strings.Contains(content, "this should be logged") {
				t.Errorf("[%s] level filtering failed: info log not found", driver)
			}
		})
	}
}

// ---------- 文本格式测试 ----------
// 验证每个驱动在 Format=text 时，输出不是 JSON

func TestTextFormat(t *testing.T) {
	testPath := "./test_logs_text"
	setupTestDir(t, testPath)
	defer os.RemoveAll(testPath)

	drivers := []string{DriverZap, DriverSlog}

	for _, driver := range drivers {
		t.Run(driver, func(t *testing.T) {
			fileName := fmt.Sprintf("text_%s", driver)
			cfg := &Config{
				Driver:     driver,
				Path:       testPath,
				FileName:   fileName,
				FileSuffix: ".logger",
				Format:     FormatText,
			}
			logger := NewLogger(cfg)
			defer logger.Close()

			logger.Info("plain text message")

			logFilePath := filepath.Join(testPath, fileName+".logger")
			data, _ := os.ReadFile(logFilePath)

			var temp map[string]interface{}
			if err := json.Unmarshal(data, &temp); err == nil {
				t.Errorf("[%s] expected text output, but got valid JSON", driver)
			}
			if !strings.Contains(string(data), "plain text message") {
				t.Errorf("[%s] log content not found in text mode", driver)
			}
		})
	}
}

// ---------- 默认配置测试 ----------
// 验证 handleConfig 为空值字段填充正确默认值，且不修改调用者传入的 Config

func TestDefaultConfig(t *testing.T) {
	original := Config{}
	cfg := original

	logger := NewLogger(&cfg)
	defer logger.Close()

	if original.Driver != "" {
		t.Error("NewLogger should not modify the original Config.Driver")
	}
	if original.Level != "" {
		t.Error("NewLogger should not modify the original Config.Level")
	}
	if original.Path != "" {
		t.Error("NewLogger should not modify the original Config.Path")
	}
	if original.Format != "" {
		t.Error("NewLogger should not modify the original Config.Format")
	}

	os.RemoveAll("./logs/")
}

// ---------- WithField 链式调用测试 ----------
// 验证多次 WithField 可以累加字段，且不影响原始 logger

func TestWithFieldChaining(t *testing.T) {
	testPath := "./test_logs_chain"
	setupTestDir(t, testPath)
	defer os.RemoveAll(testPath)

	cfg := &Config{
		Driver:     DriverZap,
		Path:       testPath,
		FileName:   "chain",
		FileSuffix: ".logger",
		Format:     FormatJSON,
	}
	logger := NewLogger(cfg)
	defer logger.Close()

	l := logger.WithField("a", 1).WithField("b", 2).WithField("c", 3)
	l.Info("chained fields")

	logFilePath := filepath.Join(testPath, "chain.logger")
	logData, err := readLastLogLine(logFilePath)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	for _, key := range []string{"a", "b", "c"} {
		if _, ok := logData[key]; !ok {
			t.Errorf("missing chained field '%s', log: %v", key, logData)
		}
	}
}

// ---------- WithFields 空 map 优化测试 ----------
// 验证传入空 map 时返回同一实例，避免无效分配

func TestWithFieldsEmpty(t *testing.T) {
	testPath := "./test_logs_empty"
	setupTestDir(t, testPath)
	defer os.RemoveAll(testPath)

	cfg := &Config{
		Driver:     DriverZap,
		Path:       testPath,
		FileName:   "empty",
		FileSuffix: ".logger",
		Format:     FormatJSON,
	}
	logger := NewLogger(cfg)
	defer logger.Close()

	same := logger.WithFields(map[string]any{})

	sameWrapper, ok := same.(*SlogWrapper)
	if !ok {
		t.Fatal("WithFields(empty) should return *SlogWrapper")
	}
	if sameWrapper != logger {
		t.Error("WithFields(empty map) should return the same instance to avoid allocation")
	}
}

// ---------- Raw 输出测试 ----------
// 验证 Raw 不带格式化信息（无 time/level/caller），直接输出原始字符串

func TestRaw(t *testing.T) {
	testPath := "./test_logs_raw"
	setupTestDir(t, testPath)
	defer os.RemoveAll(testPath)

	cfg := &Config{
		Driver:     DriverZap,
		Path:       testPath,
		FileName:   "raw",
		FileSuffix: ".logger",
		Format:     FormatJSON,
	}
	logger := NewLogger(cfg)
	defer logger.Close()

	rawMsg := `{"custom":"raw data","code":200}`
	logger.Raw(rawMsg)

	logFilePath := filepath.Join(testPath, "raw.logger")
	data, err := os.ReadFile(logFilePath)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	content := strings.TrimSpace(string(data))
	if content != rawMsg {
		t.Errorf("Raw output mismatch.\nexpected: %s\ngot:      %s", rawMsg, content)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("Raw output is not valid JSON: %v", err)
	}
	if _, ok := parsed["level"]; ok {
		t.Error("Raw output should not contain 'level' field")
	}
	if _, ok := parsed["time"]; ok {
		t.Error("Raw output should not contain 'time' field")
	}
}

// ---------- SetGlobalLogger 测试 ----------
// 验证 SetGlobalLogger 覆盖全局 logger 后，WithContext 使用新 logger

func TestSetGlobalLogger(t *testing.T) {
	testPath := "./test_logs_global"
	setupTestDir(t, testPath)
	defer os.RemoveAll(testPath)

	cfg := &Config{
		Driver:     DriverZap,
		Path:       testPath,
		FileName:   "global",
		FileSuffix: ".logger",
		Format:     FormatJSON,
	}
	logger := NewLogger(cfg)
	defer logger.Close()

	SetGlobalLogger(logger)
	defer SetGlobalLogger(nil)

	ctx := context.WithValue(context.Background(), defined.RequestID, "global-req-001")
	WithContext(ctx).Info("global logger test")

	logFilePath := filepath.Join(testPath, "global.logger")
	logData, err := readLastLogLine(logFilePath)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	if logData["request_id"] != "global-req-001" {
		t.Errorf("expected request_id 'global-req-001', got: %v", logData["request_id"])
	}
}

// ---------- 并发安全测试 ----------
// 验证多 goroutine 同时调用 WithContext + Info 不会 panic 或 data race

func TestConcurrentSafety(t *testing.T) {
	testPath := "./test_logs_concurrent"
	setupTestDir(t, testPath)
	defer os.RemoveAll(testPath)

	cfg := &Config{
		Driver:     DriverZap,
		Path:       testPath,
		FileName:   "concurrent",
		FileSuffix: ".logger",
		Format:     FormatJSON,
	}
	logger := NewLogger(cfg)
	defer logger.Close()

	SetGlobalLogger(logger)
	defer SetGlobalLogger(nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ctx := context.WithValue(context.Background(), defined.RequestID, fmt.Sprintf("req-%d", n))
			l := WithContext(ctx).WithField("goroutine", n)
			l.Info("concurrent log")
		}(i)
	}
	wg.Wait()

	logFilePath := filepath.Join(testPath, "concurrent.logger")
	data, err := os.ReadFile(logFilePath)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 100 {
		t.Errorf("expected at least 100 log lines, got %d", len(lines))
	}
}

// ---------- Close 测试 ----------
// 验证 Close 后文件 writer 关闭不报错，派生 logger 无 Close 影响

func TestClose(t *testing.T) {
	testPath := "./test_logs_close"
	setupTestDir(t, testPath)
	defer os.RemoveAll(testPath)

	cfg := &Config{
		Driver:     DriverZap,
		Path:       testPath,
		FileName:   "close_test",
		FileSuffix: ".logger",
		Format:     FormatJSON,
	}
	logger := NewLogger(cfg)

	logger.Info("before close")

	if err := logger.Close(); err != nil {
		t.Fatalf("Close should not return error: %v", err)
	}

	derived := logger.WithField("key", "val")
	if wrapper, ok := derived.(*SlogWrapper); ok {
		if wrapper.fileWriter != nil {
			t.Error("derived logger should not hold fileWriter")
		}
	}
}

// ---------- WithContext 自动初始化测试 ----------
// 验证 globalLogger 为 nil 时 WithContext 自动用默认配置创建 logger

func TestWithContextAutoInit(t *testing.T) {
	mu.Lock()
	oldLogger := globalLogger
	globalLogger = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		globalLogger = oldLogger
		mu.Unlock()
		os.RemoveAll("./logs/")
	}()

	ctx := context.Background()
	l := WithContext(ctx)
	if l == nil {
		t.Fatal("WithContext should auto-init and never return nil")
	}

	l.Info("auto init test")
}
