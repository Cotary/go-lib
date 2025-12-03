package export

import (
	"context"
)

// Writer 导出写入器接口
type Writer interface {
	// WriteHeader 写入表头
	WriteHeader(header []interface{}) error
	// WriteRow 写入一行数据
	WriteRow(row []interface{}) error
	// WriteRows 批量写入多行数据
	WriteRows(rows [][]interface{}) error
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

// 注册的写入器工厂
var writerFactories = map[string]WriterFactory{
	FormatExcel: NewExcelWriter,
	FormatCSV:   NewCSVWriter,
}

// RegisterWriter 注册自定义写入器
func RegisterWriter(format string, factory WriterFactory) {
	writerFactories[format] = factory
}

// NewWriter 根据格式创建写入器
func NewWriter(ctx context.Context, format, fileName string) (Writer, error) {
	factory, ok := writerFactories[format]
	if !ok {
		factory = writerFactories[FormatExcel] // 默认使用Excel
	}
	return factory(ctx, fileName)
}
