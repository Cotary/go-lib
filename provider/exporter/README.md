# Exporter 导出器使用说明

`provider/exporter` 是一个基于 Gin 框架的数据导出工具，支持将结构体列表导出为 Excel 或 CSV 文件。通过 `export` 标签灵活配置导出字段和数据转换。

## 快速开始

### 1. 基本使用

```go
// 定义响应结构体（必须包含 List 字段）
type UserListResponse struct {
    List []*User `json:"list"`
}

type User struct {
    ID        int64  `json:"id" export:"用户ID"`
    Name      string `json:"name" export:"用户名"`
    Email     string `json:"email" export:"邮箱"`
    CreatedAt int64  `json:"created_at" export:"创建时间,date"`
}

// 在 Gin Handler 中使用
func ListUsers(c *gin.Context) {
    // 检查是否为下载请求
    if exporter.IsDownload(c) {
        res := &UserListResponse{List: users}
        if err := exporter.NewExporter().Run(c, res); err != nil {
            // 处理错误
        }
        return
    }
    // 正常 JSON 响应
    c.JSON(200, users)
}
```

### 2. 前端请求头

| 请求头 | 值 | 说明 |
|--------|-----|------|
| `X-DOWNLOAD` | `TRUE` | 触发下载模式 |
| `X-DOWNLOAD-NAME` | `用户列表` | 自定义文件名（可选） |
| `X-EXPORT-FORMAT` | `excel` / `csv` | 导出格式，默认 `excel` |

---

## export 标签语法

```
export:"表头名称,转义函数1=参数,转义函数2=参数,..."
```

### 基本规则

| 标签值 | 说明 |
|--------|------|
| `export:"用户名"` | 导出该字段，表头为"用户名" |
| `export:"-"` | 不导出该字段 |
| 无 `export` 标签 | 不导出该字段 |

---

## 内置转义函数

### 1. `date` - 时间戳格式化

将 Unix 时间戳转换为可读的日期时间格式。

```go
type Order struct {
    // 默认格式: 2006-01-02 15:04:05
    CreatedAt int64 `export:"创建时间,date"`
    
    // 自定义格式
    UpdatedAt int64 `export:"更新时间,date=2006-01-02"`
    BirthDate int64 `export:"出生日期,date=2006年01月02日"`
}
```

| 输入值 | 参数 | 输出 |
|--------|------|------|
| `1701590400` | (无) | `2023-12-03 16:00:00` |
| `1701590400` | `2006-01-02` | `2023-12-03` |
| `0` | - | `` (空字符串) |

---

### 2. `add` - 加法运算

对数值进行加法运算，支持高精度 decimal 计算。

```go
type Product struct {
    Price float64 `export:"含税价格,add=100"` // 价格 + 100
}
```

---

### 3. `sub` - 减法运算

对数值进行减法运算。

```go
type Product struct {
    Price float64 `export:"折后价格,sub=50"` // 价格 - 50
}
```

---

### 4. `mul` - 乘法运算

对数值进行乘法运算。

```go
type Product struct {
    Price    float64 `export:"美元价格,mul=0.14"`  // 价格 × 0.14
    Quantity int     `export:"总数量,mul=2"`       // 数量 × 2
}
```

---

### 5. `div` - 除法运算

对数值进行除法运算（除数为 0 时返回错误）。

```go
type Product struct {
    PriceCent int64 `export:"价格(元),div=100"` // 分 ÷ 100 = 元
}
```

---

### 6. `floor` - 向下取整

对数值进行向下取整。

```go
type Stats struct {
    AvgScore float64 `export:"平均分,floor"` // 85.7 → 85
}
```

---

### 7. `format` - 字符串格式化

使用 `%s` 占位符进行字符串格式化。

```go
type User struct {
    Phone string `export:"联系电话,format=+86 %s"`  // 输出: +86 13800138000
    ID    int64  `export:"用户编号,format=U%s"`     // 输出: U12345
}
```

---

### 8. `enum` - 枚举映射

将值映射为可读的枚举文本。

```go
type Order struct {
    // 状态值映射: 1→待支付, 2→已支付, 3→已发货, 4→已完成
    Status int `export:"订单状态,enum=1:待支付 2:已支付 3:已发货 4:已完成"`
    
    // 字符串枚举
    Gender string `export:"性别,enum=M:男 F:女 U:未知"`
}
```

**语法**: `enum=值1:显示文本1 值2:显示文本2 ...`（空格分隔）

---

## 转义函数链式调用

可以组合多个转义函数，按顺序依次执行：

```go
type Order struct {
    // 先除以100转为元，再加上10元运费
    TotalCent int64 `export:"总金额(含运费),div=100,add=10"`
    
    // 先打8折，再取整
    Amount float64 `export:"金额,mul=0.8,floor"`
}
```

---

## 特殊类型支持

### 1. `Export()` 方法（最高优先级）

如果字段类型实现了 `Export() string` 方法，导出时会优先调用该方法：

```go
type Address struct {
    Province string
    City     string
    District string
}

func (a Address) Export() string {
    return a.Province + a.City + a.District
}

type User struct {
    Address Address `export:"地址"` // 输出: 广东省深圳市南山区
}
```

### 2. `String()` 方法

如果字段类型实现了 `String() string` 方法（如 `fmt.Stringer`），且没有 `Export()` 方法时，导出时会调用该方法：

```go
type Status int

func (s Status) String() string {
    switch s {
    case 1: return "启用"
    case 0: return "禁用"
    default: return "未知"
    }
}

type User struct {
    Status Status `export:"状态"` // 输出: 启用/禁用
}
```

**优先级**: `Export()` > `String()` > 默认转换

---

## 自定义转义函数

可以注册自定义的转义函数：

```go
func init() {
    // 注册自定义函数
    exporter.RegisterEscapeFunc("mask", maskPhone)
}

// 手机号脱敏: 13800138000 → 138****8000
func maskPhone(origin any, arg string) (any, error) {
    phone, err := utils.AnyToAny[string](origin)
    if err != nil {
        return nil, err
    }
    if len(phone) != 11 {
        return phone, nil
    }
    return phone[:3] + "****" + phone[7:], nil
}

// 使用
type User struct {
    Phone string `export:"手机号,mask"`
}
```

### 函数签名

```go
type EscapeFunc func(origin any, arg string) (any, error)
```

- `origin`: 字段原始值
- `arg`: 标签中 `=` 后的参数值
- 返回: 转换后的值和错误

---

## 复杂类型处理

| 类型 | 处理方式 |
|------|----------|
| `int`, `float`, `bool`, `string` | 直接输出 |
| `[]T`, `map[K]V`, `struct` | JSON 序列化后输出 |
| 指针类型 | 自动解引用 |
| 零值/nil | 输出空字符串 |

---

## 完整示例

```go
type OrderExportResponse struct {
    List []*Order `json:"list"`
}

type Order struct {
    ID          int64   `json:"id" export:"订单ID"`
    UserName    string  `json:"user_name" export:"用户名"`
    Phone       string  `json:"phone" export:"手机号,mask"`
    Status      int     `json:"status" export:"状态,enum=0:待支付 1:已支付 2:已发货 3:已完成 4:已取消"`
    AmountCent  int64   `json:"amount_cent" export:"订单金额(元),div=100"`
    CreatedAt   int64   `json:"created_at" export:"下单时间,date=2006-01-02 15:04:05"`
    Address     Address `json:"address" export:"收货地址"` // 调用 Address.Export() 方法
    InternalID  string  `json:"-" export:"-"` // 不导出
}

type Address struct {
    Province string `json:"province"`
    City     string `json:"city"`
    Detail   string `json:"detail"`
}

func (a Address) Export() string {
    return a.Province + a.City + a.Detail
}

func ExportOrders(c *gin.Context) {
    if !exporter.IsDownload(c) {
        c.JSON(400, gin.H{"error": "请使用导出功能"})
        return
    }
    
    orders := getOrders() // 获取订单数据
    res := &OrderExportResponse{List: orders}
    
    if err := exporter.NewExporter().Run(c, res); err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
}
```

---

## 注意事项

1. **响应结构必须包含 `List` 字段**，且为切片类型
2. 转义函数按声明顺序**链式执行**，前一个的输出是后一个的输入
3. 类型缓存会在首次导出后生效，相同类型的后续导出性能更好
4. 批量写入默认每 100 行刷新一次，适合大数据量导出
5. `Export()` 方法优先级高于 `String()` 方法
