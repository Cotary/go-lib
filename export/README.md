# export

通用数据导出包，支持 Excel（`.xlsx`）和 CSV 格式，基于统一的 `Writer` 接口。

## 特性

- **流式 Excel 写入** — 基于 `excelize.StreamWriter`，万行数据低内存占用
- **WriteTo** — 支持写入任意 `io.Writer`（HTTP Response、bytes.Buffer、S3 上传流等）
- **Context 感知** — `WriteRows` 批量写入时自动检查 context 取消
- **工厂模式** — `RegisterWriter` 可扩展自定义格式
- **struct 映射** — 泛型 `StructSliceToRows` 通过 struct tag 自动生成表头和数据行
- **安全校验** — 文件名路径穿越防护

## 快速开始

### 基础用法

```go
ctx := context.Background()

// 创建 Excel 写入器
w, err := export.NewWriter(ctx, export.FormatExcel, "report")
if err != nil {
    log.Fatal(err)
}
defer w.Close()

// 写入表头
w.WriteHeader([]interface{}{"姓名", "年龄", "城市"})

// 逐行写入
w.WriteRow([]interface{}{"张三", 28, "北京"})

// 批量写入
rows := [][]interface{}{
    {"李四", 35, "上海"},
    {"王五", 22, "深圳"},
}
w.WriteRows(rows)
```

### 使用 struct 映射

通过 struct tag 自动提取列名和数据，避免手动构造 `[]interface{}`：

```go
type Order struct {
    ID     int     `export:"订单号"`
    Item   string  `export:"商品"`
    Amount float64 `export:"金额"`
    inner  string  // 未导出，自动跳过
}

orders := []Order{
    {ID: 1001, Item: "笔记本电脑", Amount: 5999.00},
    {ID: 1002, Item: "鼠标", Amount: 59.90},
}

header, rows := export.StructSliceToRows(orders, "export")

w, _ := export.NewWriter(ctx, export.FormatExcel, "orders")
defer w.Close()

w.WriteHeader(header)
w.WriteRows(rows)
```

### HTTP 文件下载（Gin 示例）

`WriteTo` 可以直接将内容写入 HTTP Response，无需先保存到磁盘：

```go
func ExportHandler(c *gin.Context) {
    w, err := export.NewWriter(c.Request.Context(), export.FormatExcel, "report")
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    w.WriteHeader([]interface{}{"ID", "Name"})
    w.WriteRows(data)

    c.Header("Content-Disposition", "attachment; filename="+w.FileName())
    c.Header("Content-Type", w.ContentType())
    
    if err := w.WriteTo(c.Writer); err != nil {
        log.Printf("export write error: %v", err)
    }
}
```

> **注意**: 调用 `WriteTo` 后，Writer 进入只读状态，不可再写入新数据。

### CSV 格式

用法与 Excel 完全一致，只需切换 format：

```go
w, err := export.NewWriter(ctx, export.FormatCSV, "data")
```

CSV 会自动写入 UTF-8 BOM，确保 Excel 打开时中文不乱码。

## 自定义写入器

通过 `RegisterWriter` 注册自定义格式：

```go
func init() {
    export.RegisterWriter("tsv", func(ctx context.Context, fileName string) (export.Writer, error) {
        // 实现 Writer 接口
        return &TSVWriter{...}, nil
    })
}

// 使用
w, _ := export.NewWriter(ctx, "tsv", "output")
```

## 注意事项

| 项目 | 说明 |
|------|------|
| 格式校验 | 传入不支持的 format 会返回 error，不会静默降级 |
| 文件名安全 | 空文件名、绝对路径、路径穿越（`../`）均会被拦截 |
| Context 取消 | `WriteRows` 每行写入前检查 `ctx.Done()`，`WriteRow` 不检查（单行无需） |
| WriteTo 后状态 | Excel 调用 `WriteTo` 后内部流已 flush，再调用 `WriteRow` 返回 `ErrWriterFlushed` |
| Close 行为 | Excel: flush stream → SaveAs → Close file；CSV: Flush → Close file |
| 并发安全 | `RegisterWriter` / `NewWriter` 内部有读写锁保护，可安全并发调用 |
