package export

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
)

// Writer 导出写入器接口
type Writer interface {
	// WriteHeader 写入表头
	WriteHeader(header []interface{}) error
	// WriteRow 写入一行数据
	WriteRow(row []interface{}) error
	// WriteRows 批量写入多行数据
	WriteRows(rows [][]interface{}) error
	// WriteTo 将内容写入 io.Writer（如 HTTP Response、bytes.Buffer）
	WriteTo(w io.Writer) error
	// Close 关闭并保存文件
	Close() error
	// FileName 获取文件名
	FileName() string
	// ContentType 获取Content-Type
	ContentType() string
}

// WriterFactory 写入器工厂函数类型
type WriterFactory func(ctx context.Context, fileName string) (Writer, error)

// 支持的格式常量
const (
	FormatExcel = "excel"
	FormatCSV   = "csv"
)

var (
	mu              sync.RWMutex
	writerFactories = map[string]WriterFactory{
		FormatExcel: NewExcelWriter,
		FormatCSV:   NewCSVWriter,
	}
)

// RegisterWriter 注册自定义写入器
func RegisterWriter(format string, factory WriterFactory) {
	mu.Lock()
	defer mu.Unlock()
	writerFactories[format] = factory
}

// NewWriter 根据格式创建写入器
func NewWriter(ctx context.Context, format, fileName string) (Writer, error) {
	mu.RLock()
	factory, ok := writerFactories[format]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("export: unsupported format: %q", format)
	}
	return factory(ctx, fileName)
}

// ValidateFileName 校验文件名，防止路径穿越和非法路径
func ValidateFileName(name string) error {
	if name == "" {
		return fmt.Errorf("export: file name cannot be empty")
	}
	cleaned := filepath.Clean(name)
	if filepath.IsAbs(cleaned) {
		return fmt.Errorf("export: absolute path not allowed: %s", name)
	}
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("export: path traversal not allowed: %s", name)
	}
	return nil
}
