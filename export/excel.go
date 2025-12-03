package export

import (
	"context"

	"github.com/xuri/excelize/v2"
)

// ExcelWriter Excel写入器
type ExcelWriter struct {
	ctx      context.Context
	file     *excelize.File
	fileName string
	sheet    string
	rowIndex int
}

// NewExcelWriter 创建Excel写入器
func NewExcelWriter(ctx context.Context, fileName string) (Writer, error) {
	f := excelize.NewFile()
	return &ExcelWriter{
		ctx:      ctx,
		file:     f,
		fileName: fileName + ".xlsx",
		sheet:    "Sheet1",
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

// WriteRows 批量写入多行数据
func (w *ExcelWriter) WriteRows(rows [][]interface{}) error {
	for _, row := range rows {
		if err := w.writeRow(row); err != nil {
			return err
		}
	}
	return nil
}

func (w *ExcelWriter) writeRow(data []interface{}) error {
	for colIndex, cellValue := range data {
		cell, err := excelize.CoordinatesToCellName(colIndex+1, w.rowIndex)
		if err != nil {
			return err
		}
		if err := w.file.SetCellValue(w.sheet, cell, cellValue); err != nil {
			return err
		}
	}
	w.rowIndex++
	return nil
}

// Close 关闭并保存文件
func (w *ExcelWriter) Close() error {
	return w.file.SaveAs(w.fileName)
}

// FileName 获取文件名
func (w *ExcelWriter) FileName() string {
	return w.fileName
}

// ContentType 获取Content-Type
func (w *ExcelWriter) ContentType() string {
	return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}
