package log

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Cotary/go-lib/common/defined"
)

// 定义测试用的 Context Key
type contextKey string

const requestIDKey contextKey = "request_id"

// 辅助函数：清理测试日志目录
func setupTestDir(t *testing.T, path string) {
	err := os.RemoveAll(path)
	if err != nil {
		t.Fatalf("failed to clear test dir: %v", err)
	}
	err = os.MkdirAll(path, 0755)
	if err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}
}

// 辅助函数：读取最后一行日志并解析为 Map
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

// TestConsistency 测试三个驱动的一致性
func TestConsistency(t *testing.T) {
	testPath := "./test_logs"
	setupTestDir(t, testPath)
	defer os.RemoveAll(testPath)

	drivers := []string{DriverZap, DriverZerolog, DriverSlog}

	for _, driver := range drivers {
		t.Run(string(driver), func(t *testing.T) {
			fileName := fmt.Sprintf("test_%s", driver)
			cfg := &Config{
				Driver:     driver,
				Level:      "debug",
				Path:       testPath,
				FileName:   fileName,
				FileSuffix: ".logger",
				ShowFile:   true,
				Format:     "json",
			}

			globalLogger = NewLogger(cfg)

			// 测试数据
			msg := "consistency check"
			cost := 1234 * time.Millisecond // 1.234s

			// 模拟 Context
			ctx := context.WithValue(context.Background(), defined.RequestID, "req-123456")
			// 模拟 WithContext 逻辑（需根据实际定义的 key 调整 WithContext 源码）
			l := WithContext(ctx).WithFields(map[string]any{
				"cost":       cost,
				"request_id": "req-123",
			})

			l.Info(msg)

			// 验证结果
			logFilePath := filepath.Join(testPath, fileName+".logger")
			logData, err := readLastLogLine(logFilePath)
			if err != nil {
				t.Fatalf("[%s] failed to read logger: %v", driver, err)
			}

			// 1. 检查时间 Key 是否为 "time"
			if _, ok := logData["time"]; !ok {
				t.Errorf("[%s] missing 'time' key", driver)
			}

			// 2. 检查消息内容
			if logData["msg"] != msg && logData["message"] != msg {
				// 兼容处理：slog/zap 通常用 msg，zerolog 可能配置不同，但在本封装中应统一
				t.Errorf("[%s] unexpected msg: %v", driver, logData["msg"])
			}

			// 3. 检查 Duration 是否为秒 (Float64)
			costVal, ok := logData["cost"].(float64)
			if !ok || costVal != 1.234 {
				t.Errorf("[%s] duration should be 1.234 seconds, got: %v", driver, logData["cost"])
			}

			// 4. 检查 ShowFile (行号) 是否包含本文件名
			// 注意：slog 的 key 是 "source"，zap 是 "caller"，zerolog 是 "caller"
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

			// 5. 检查级别是否正确
			if !strings.EqualFold(fmt.Sprint(logData["level"]), "info") {
				t.Errorf("[%s] unexpected level: %v", driver, logData["level"])
			}
		})
	}
}

// TestLevelFiltering 测试日志级别过滤是否生效
func TestLevelFiltering(t *testing.T) {
	testPath := "./test_logs_level"
	setupTestDir(t, testPath)
	defer os.RemoveAll(testPath)

	cfg := &Config{
		Driver:     DriverZap,
		Level:      "info", // 设置为 Info，应该过滤掉 Debug
		Path:       testPath,
		FileName:   "level_test",
		FileSuffix: ".logger",
		Format:     "json",
	}
	globalLogger = NewLogger(cfg)

	globalLogger.Debug("this should not be logged")
	globalLogger.Info("this should be logged")

	logFilePath := filepath.Join(testPath, "level_test.logger")
	data, _ := os.ReadFile(logFilePath)
	content := string(data)

	if strings.Contains(content, "this should not be logged") {
		t.Error("Level filtering failed: Debug logger found when level set to Info")
	}
	if !strings.Contains(content, "this should be logged") {
		t.Error("Level filtering failed: Info logger not found")
	}
}

// TestTextFormat 测试文本格式是否生效
func TestTextFormat(t *testing.T) {
	testPath := "./test_logs_text"
	setupTestDir(t, testPath)
	defer os.RemoveAll(testPath)

	cfg := &Config{
		Driver:     DriverSlog,
		Path:       testPath,
		FileName:   "text_test",
		FileSuffix: ".logger",
		Format:     "text", // 设置为 text
	}
	globalLogger = NewLogger(cfg)

	globalLogger.Info("plain text message")
	globalLogger.Raw("plain text message")

	logFilePath := filepath.Join(testPath, "text_test.logger")
	data, _ := os.ReadFile(logFilePath)
	content := string(data)

	// 如果是文本格式，不应该能被解析为 JSON
	var temp map[string]interface{}
	err := json.Unmarshal(data, &temp)
	if err == nil {
		t.Error("Format check failed: expected text output, but got valid JSON")
	}
	if !strings.Contains(content, "plain text message") {
		t.Error("Log content not found in text mode")
	}
}
