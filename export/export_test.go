package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/xuri/excelize/v2"
)

// ——————————————————————————————————
// Excel 基础流程
// ——————————————————————————————————

func TestExcelWriter_BasicFlow(t *testing.T) {
	ctx := context.Background()
	w, err := NewExcelWriter(ctx, "test_basic")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(w.FileName())

	if err := w.WriteHeader([]interface{}{"Name", "Age", "City"}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteRow([]interface{}{"Alice", 30, "Beijing"}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteRow([]interface{}{"Bob", 25, "Shanghai"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// 读回验证
	f, err := excelize.OpenFile(w.FileName())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	rows, err := f.GetRows("Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0][0] != "Name" || rows[0][1] != "Age" || rows[0][2] != "City" {
		t.Fatalf("header mismatch: %v", rows[0])
	}
	if rows[1][0] != "Alice" {
		t.Fatalf("row 1 name mismatch: %v", rows[1])
	}
}

// ——————————————————————————————————
// CSV 基础流程
// ——————————————————————————————————

func TestCSVWriter_BasicFlow(t *testing.T) {
	ctx := context.Background()
	w, err := NewCSVWriter(ctx, "test_basic")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(w.FileName())

	if err := w.WriteHeader([]interface{}{"姓名", "年龄"}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteRow([]interface{}{"张三", 28}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(w.FileName())
	if err != nil {
		t.Fatal(err)
	}
	// 去掉 BOM
	content := strings.TrimPrefix(string(data), "\xEF\xBB\xBF")
	r := csv.NewReader(strings.NewReader(content))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0][0] != "姓名" || records[0][1] != "年龄" {
		t.Fatalf("header mismatch: %v", records[0])
	}
	if records[1][0] != "张三" {
		t.Fatalf("row mismatch: %v", records[1])
	}
}

// ——————————————————————————————————
// WriteRows 批量写入
// ——————————————————————————————————

func TestExcelWriter_WriteRows(t *testing.T) {
	ctx := context.Background()
	w, err := NewExcelWriter(ctx, "test_rows")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(w.FileName())

	_ = w.WriteHeader([]interface{}{"ID", "Value"})
	rows := make([][]interface{}, 100)
	for i := range rows {
		rows[i] = []interface{}{i + 1, i * 10}
	}
	if err := w.WriteRows(rows); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := excelize.OpenFile(w.FileName())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	result, _ := f.GetRows("Sheet1")
	// 1 header + 100 data
	if len(result) != 101 {
		t.Fatalf("expected 101 rows, got %d", len(result))
	}
}

func TestCSVWriter_WriteRows(t *testing.T) {
	ctx := context.Background()
	w, err := NewCSVWriter(ctx, "test_rows")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(w.FileName())

	_ = w.WriteHeader([]interface{}{"ID", "Value"})
	rows := make([][]interface{}, 50)
	for i := range rows {
		rows[i] = []interface{}{i, i * 2}
	}
	if err := w.WriteRows(rows); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(w.FileName())
	content := strings.TrimPrefix(string(data), "\xEF\xBB\xBF")
	r := csv.NewReader(strings.NewReader(content))
	records, _ := r.ReadAll()
	if len(records) != 51 {
		t.Fatalf("expected 51 records, got %d", len(records))
	}
}

// ——————————————————————————————————
// WriteTo（写入 io.Writer）
// ——————————————————————————————————

func TestExcelWriter_WriteTo(t *testing.T) {
	ctx := context.Background()
	w, err := NewExcelWriter(ctx, "test_writeto")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(w.FileName())

	_ = w.WriteHeader([]interface{}{"Col1"})
	_ = w.WriteRow([]interface{}{"Data1"})

	var buf bytes.Buffer
	if err := w.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Fatal("WriteTo produced empty output")
	}

	// 验证 WriteTo 后不可再写入
	if err := w.WriteRow([]interface{}{"Should fail"}); err != ErrWriterFlushed {
		t.Fatalf("expected ErrWriterFlushed, got %v", err)
	}

	// 验证写出的 xlsx 可被解析
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, _ := f.GetRows("Sheet1")
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows from WriteTo output, got %d", len(rows))
	}
}

func TestCSVWriter_WriteTo(t *testing.T) {
	ctx := context.Background()
	w, err := NewCSVWriter(ctx, "test_writeto")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(w.FileName())

	_ = w.WriteHeader([]interface{}{"H1", "H2"})
	_ = w.WriteRow([]interface{}{"A", "B"})

	var buf bytes.Buffer
	if err := w.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Fatal("WriteTo produced empty output")
	}

	content := strings.TrimPrefix(buf.String(), "\xEF\xBB\xBF")
	r := csv.NewReader(strings.NewReader(content))
	records, _ := r.ReadAll()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
}

// ——————————————————————————————————
// NewWriter 工厂 & 未知格式报错
// ——————————————————————————————————

func TestNewWriter_Formats(t *testing.T) {
	ctx := context.Background()

	// Excel
	w1, err := NewWriter(ctx, FormatExcel, "test_factory_excel")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(w1.FileName())
	_ = w1.Close()

	if w1.ContentType() != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatal("wrong content type for excel")
	}

	// CSV
	w2, err := NewWriter(ctx, FormatCSV, "test_factory_csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(w2.FileName())
	_ = w2.Close()

	if w2.ContentType() != "text/csv; charset=utf-8" {
		t.Fatal("wrong content type for csv")
	}
}

func TestNewWriter_UnknownFormat(t *testing.T) {
	_, err := NewWriter(context.Background(), "pdf", "test")
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ——————————————————————————————————
// RegisterWriter 并发安全
// ——————————————————————————————————

func TestRegisterWriter_ConcurrentSafety(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			RegisterWriter("concurrent_test", NewCSVWriter)
		}(i)
		go func(n int) {
			defer wg.Done()
			_, _ = NewWriter(context.Background(), FormatExcel, "concurrent_test")
		}(i)
	}
	wg.Wait()
	// 无 panic 即通过
	// 清理可能生成的文件
	os.Remove("concurrent_test.xlsx")
}

// ——————————————————————————————————
// ValidateFileName
// ——————————————————————————————————

func TestValidateFileName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"report", false},
		{"data/report", false},
		{"", true},
		{"../etc/passwd", true},
		{"foo/../../bar", true},
	}

	for _, tt := range tests {
		err := ValidateFileName(tt.name)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateFileName(%q) error=%v, wantErr=%v", tt.name, err, tt.wantErr)
		}
	}

	// 平台相关：Windows 绝对路径
	if err := ValidateFileName("C:\\Users\\test"); err == nil {
		t.Error("expected error for absolute path")
	}
}

// ——————————————————————————————————
// Context 取消
// ——————————————————————————————————

func TestExcelWriter_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w, err := NewExcelWriter(ctx, "test_ctx_cancel")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(w.FileName())

	cancel()

	rows := make([][]interface{}, 1000)
	for i := range rows {
		rows[i] = []interface{}{i}
	}
	err = w.WriteRows(rows)
	if err == nil {
		t.Fatal("expected context canceled error")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	_ = w.Close()
}

func TestCSVWriter_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w, err := NewCSVWriter(ctx, "test_ctx_cancel_csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(w.FileName())

	cancel()

	rows := make([][]interface{}, 1000)
	for i := range rows {
		rows[i] = []interface{}{i}
	}
	err = w.WriteRows(rows)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	_ = w.Close()
}

// ——————————————————————————————————
// StructSliceToRows 泛型辅助
// ——————————————————————————————————

type testUser struct {
	Name    string `export:"姓名"`
	Age     int    `export:"年龄"`
	Email   string `export:"邮箱"`
	secret  string // 未导出，应被跳过
	Skipped string // 无 export tag，应被跳过
	Ignored string `export:"-"`
}

func TestStructSliceToRows(t *testing.T) {
	users := []testUser{
		{Name: "张三", Age: 28, Email: "zhang@example.com", secret: "x", Skipped: "y"},
		{Name: "李四", Age: 35, Email: "li@example.com"},
	}

	header, rows := StructSliceToRows(users, "export")

	if len(header) != 3 {
		t.Fatalf("expected 3 header columns, got %d: %v", len(header), header)
	}
	if header[0] != "姓名" || header[1] != "年龄" || header[2] != "邮箱" {
		t.Fatalf("header mismatch: %v", header)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0][0] != "张三" || rows[0][1] != 28 || rows[0][2] != "zhang@example.com" {
		t.Fatalf("row 0 mismatch: %v", rows[0])
	}
}

func TestStructSliceToRows_Empty(t *testing.T) {
	header, rows := StructSliceToRows([]testUser{}, "export")
	if len(header) != 3 {
		t.Fatal("header should still be generated for empty slice")
	}
	if len(rows) != 0 {
		t.Fatal("rows should be empty")
	}
}

func TestStructSliceToRows_NonStruct(t *testing.T) {
	header, rows := StructSliceToRows([]int{1, 2, 3}, "export")
	if header != nil || rows != nil {
		t.Fatal("expected nil for non-struct type")
	}
}

// ——————————————————————————————————
// WriteTo + 完整写入 Excel 流程（模拟 HTTP 下载）
// ——————————————————————————————————

func TestExcelWriter_WriteToHTTPSimulation(t *testing.T) {
	type Order struct {
		ID     int     `export:"订单号"`
		Item   string  `export:"商品"`
		Amount float64 `export:"金额"`
	}

	orders := []Order{
		{ID: 1001, Item: "笔记本电脑", Amount: 5999.00},
		{ID: 1002, Item: "鼠标", Amount: 59.90},
	}

	header, rows := StructSliceToRows(orders, "export")

	ctx := context.Background()
	w, err := NewExcelWriter(ctx, "test_http_sim")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(w.FileName())

	if err := w.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteRows(rows); err != nil {
		t.Fatal(err)
	}

	// 模拟写入 HTTP Response body
	var httpBody bytes.Buffer
	if err := w.WriteTo(&httpBody); err != nil {
		t.Fatal(err)
	}

	// 验证可被 excelize 解析
	f, err := excelize.OpenReader(bytes.NewReader(httpBody.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	result, _ := f.GetRows("Sheet1")
	if len(result) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result))
	}
	if result[0][0] != "订单号" {
		t.Fatalf("header[0] mismatch: %v", result[0][0])
	}
}

// ——————————————————————————————————
// Benchmark
// ——————————————————————————————————

func BenchmarkExcelWriter_1000Rows(b *testing.B) {
	rows := make([][]interface{}, 1000)
	for i := range rows {
		rows[i] = []interface{}{i, "name", 3.14, true}
	}

	for b.Loop() {
		w, err := NewExcelWriter(context.Background(), "bench_excel")
		if err != nil {
			b.Fatal(err)
		}
		_ = w.WriteHeader([]interface{}{"ID", "Name", "Score", "Active"})
		_ = w.WriteRows(rows)
		_ = w.WriteTo(io.Discard)
		_ = w.Close()
		os.Remove(w.FileName())
	}
}

func BenchmarkCSVWriter_1000Rows(b *testing.B) {
	rows := make([][]interface{}, 1000)
	for i := range rows {
		rows[i] = []interface{}{i, "name", 3.14, true}
	}

	for b.Loop() {
		w, err := NewCSVWriter(context.Background(), "bench_csv")
		if err != nil {
			b.Fatal(err)
		}
		_ = w.WriteHeader([]interface{}{"ID", "Name", "Score", "Active"})
		_ = w.WriteRows(rows)
		_ = w.Close()
		os.Remove(w.FileName())
	}
}
