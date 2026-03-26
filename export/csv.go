package export

import (
	"context"
	"encoding/csv"
	"io"
	"os"

	"github.com/Cotary/go-lib/common/utils"
)

// CSVWriter CSV写入器
type CSVWriter struct {
	ctx      context.Context
	file     *os.File
	writer   *csv.Writer
	fileName string
}

// NewCSVWriter 创建CSV写入器
func NewCSVWriter(ctx context.Context, fileName string) (Writer, error) {
	if err := ValidateFileName(fileName); err != nil {
		return nil, err
	}
	fullName := fileName + ".csv"
	file, err := os.Create(fullName)
	if err != nil {
		return nil, err
	}

	// UTF-8 BOM，确保 Excel 正确识别中文
	if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		file.Close()
		return nil, err
	}

	return &CSVWriter{
		ctx:      ctx,
		file:     file,
		writer:   csv.NewWriter(file),
		fileName: fullName,
	}, nil
}

// WriteHeader 写入表头
func (w *CSVWriter) WriteHeader(header []interface{}) error {
	return w.writeRow(header)
}

// WriteRow 写入一行数据
func (w *CSVWriter) WriteRow(row []interface{}) error {
	return w.writeRow(row)
}

// WriteRows 批量写入多行数据，支持 context 取消
func (w *CSVWriter) WriteRows(rows [][]interface{}) error {
	for _, row := range rows {
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		default:
		}
		if err := w.writeRow(row); err != nil {
			return err
		}
	}
	return nil
}

func (w *CSVWriter) writeRow(data []interface{}) error {
	record := make([]string, len(data))
	for i, v := range data {
		record[i] = utils.AnyToString(v)
	}
	return w.writer.Write(record)
}

// WriteTo 将 CSV 内容写入 io.Writer，调用后不可再写入数据
func (w *CSVWriter) WriteTo(wr io.Writer) error {
	w.writer.Flush()
	if err := w.writer.Error(); err != nil {
		return err
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(wr, w.file)
	return err
}

// Close 关闭并保存文件
func (w *CSVWriter) Close() error {
	w.writer.Flush()
	if err := w.writer.Error(); err != nil {
		w.file.Close()
		return err
	}
	return w.file.Close()
}

// FileName 获取文件名
func (w *CSVWriter) FileName() string {
	return w.fileName
}

// ContentType 获取Content-Type
func (w *CSVWriter) ContentType() string {
	return "text/csv; charset=utf-8"
}
