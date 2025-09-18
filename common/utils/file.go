package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AppendLog 追加日志到指定文件（自动创建目录和文件）
func AppendLog(filePath string, data map[string]string) error {
	logEntry := fmt.Sprintf("\n%s:\n", time.Now().Format(time.DateTime))
	for key, val := range data {
		logEntry += fmt.Sprintf("%s: %s\n", key, val)
	}
	return AppendToFile(filePath, logEntry)
}

// AppendToFile 向文件追加内容（自动创建目录和文件）
func AppendToFile(filePath, content string) error {
	if err := EnsureFileExists(filePath); err != nil {
		return err
	}

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(content + "\n")
	return err
}

// EnsureFileExists 确保文件及其目录存在
func EnsureFileExists(filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	if !PathExists(filePath) {
		f, err := os.Create(filePath)
		if err != nil {
			return err
		}
		defer f.Close()
	}
	return nil
}

// PathExists 判断路径是否存在
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}
