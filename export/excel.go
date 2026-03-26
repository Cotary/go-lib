package export

import (
	"context"
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"
)

var ErrWriterFlushed = fmt.Errorf("export: writer already flushed or closed")

// ExcelWriter Excel流式写入器
type ExcelWriter struct {
	ctx      context.Context
	file     *excelize.File
	sw       *excelize.StreamWriter
	fileName string
	sheet    string
	rowIndex int
	flushed  bool
}

// NewExcelWriter 创建Excel写入器
func NewExcelWriter(ctx context.Context, fileName string) (Writer, error) {
	if err := ValidateFileName(fileName); err != nil {
		return nil, err
	}
	f := excelize.NewFile()
	sheet := "Sheet1"
	sw, err := f.NewStreamWriter(sheet)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("export: create stream writer: %w", err)
	}
	return &ExcelWriter{
		ctx:      ctx,
		file:     f,
		sw:       sw,
		fileName: fileName + ".xlsx",
		sheet:    sheet,
		rowIndex: 1,
	}, nil
}

// WriteHeader 写入表头
func (w *ExcelWriter) WriteHeader(header []interface{}) error {
	return w.writeRow(header)
}

// WriteRow 写入一行数据
func (w *ExcelWriter) WriteRow(row []interface{}) error {
	return w.writeRow(row)
}

// WriteRows 批量写入多行数据，支持 context 取消
func (w *ExcelWriter) WriteRows(rows [][]interface{}) error {
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

func (w *ExcelWriter) writeRow(data []interface{}) error {
	if w.flushed {
		return ErrWriterFlushed
	}
	cell, err := excelize.CoordinatesToCellName(1, w.rowIndex)
	if err != nil {
		return err
	}
	if err := w.sw.SetRow(cell, data); err != nil {
		return err
	}
	w.rowIndex++
	return nil
}

func (w *ExcelWriter) flush() error {
	if !w.flushed {
		w.flushed = true
		return w.sw.Flush()
	}
	return nil
}

// WriteTo 将 Excel 内容写入 io.Writer，调用后不可再写入数据
func (w *ExcelWriter) WriteTo(wr io.Writer) error {
	if err := w.flush(); err != nil {
		return err
	}
	return w.file.Write(wr)
}

// Close 关闭并保存文件
func (w *ExcelWriter) Close() error {
	if err := w.flush(); err != nil {
		w.file.Close()
		return err
	}
	if err := w.file.SaveAs(w.fileName); err != nil {
		w.file.Close()
		return err
	}
	return w.file.Close()
}

// FileName 获取文件名
func (w *ExcelWriter) FileName() string {
	return w.fileName
}

// ContentType 获取Content-Type
func (w *ExcelWriter) ContentType() string {
	return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}
