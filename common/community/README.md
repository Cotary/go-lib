# community — 通用请求/响应类型

`community` 包提供业务层通用的请求、响应结构体，被多个业务项目引用。

## TimeRange 时间范围

`TimeRange` 支持多种时间格式作为查询条件，通过 `TimeRangeType` 字段声明 `StartTime` / `EndTime` 的值格式。

### 支持的 TimeRangeType

| 常量 | 值 | 说明 | 示例 |
|---|---|---|---|
| `TimeRangeTimestamp` | `"timestamp"` | 10位秒级 Unix 时间戳（默认） | `1713100800` |
| `TimeRangeTimestampMs` | `"timestamp_ms"` | 13位毫秒级 Unix 时间戳 | `1713100800000` |
| `TimeRangeYearMonth` | `"year_month"` | yyyyMM 格式 | `202604` |
| `TimeRangeDate` | `"date"` | yyyyMMdd 格式 | `20260414` |

`TimeRangeType` 底层为 `string`，gin 的 `ShouldBind` 可直接从 URL query 参数绑定，无需额外处理。

### 快速开始

```go
// 前端请求：?start_time=20260414&end_time=20260414&time_range_type=date
type MyRequest struct {
    community.TimeRange
}

func handler(c *gin.Context) {
    var req MyRequest
    c.ShouldBind(&req)

    // 不传时区，默认使用系统本地时区 (time.Local)
    start, end, err := req.TimeRange.Parse()

    // 传 string 时区名
    start, end, err = req.TimeRange.Parse("Asia/Shanghai")
    // start = 2026-04-14 00:00:00 +0800
    // end   = 2026-04-14 23:59:59.999999999 +0800

    // 传 *time.Location 变量
    loc, _ := time.LoadLocation("America/New_York")
    start, end, err = req.TimeRange.Parse(loc)

    // 也可以拿 *carbon.Carbon 做后续链式操作
    startC, endC, err := req.TimeRange.ParseCarbon("Asia/Shanghai")
    fmt.Println(startC.DiffInDays(endC))
}
```

### 闭区间语义

EndTime 为闭区间（包含当天/当月）：

- `year_month`：`EndTime=202604` → 2026-04-30 23:59:59.999999999
- `date`：`EndTime=20260414` → 2026-04-14 23:59:59.999999999
- `timestamp` / `timestamp_ms`：直接使用，不做扩展

### 时区处理

时区通过 `Parse` / `ParseCarbon` 的可选参数传入，支持两种形式：

- `string`：IANA 时区名，如 `"Asia/Shanghai"`
- `*time.Location`：直接传变量，如 `time.UTC`、自定义 `loc`

不传时默认使用系统本地时区（`time.Local`）。

- **timestamp / timestamp_ms**：时区仅影响返回的 `time.Time` 的 Location，不改变绝对时刻
- **year_month / date**：时区决定绝对时刻。`20260414` 在 `Asia/Shanghai` 是 `+0800`，在 `UTC` 是 `+0000`，两者差8小时
