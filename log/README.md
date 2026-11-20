# log

基于标准库 `log/slog` 的日志包：**Logger 就是 `*slog.Logger`，不做任何包装**。

- 格式化、级别过滤、脱敏、ctx 字段、告警、采样全部在 Handler 链里，实现在子包 `log/handler`；
- 输出端（stdout + lumberjack 文件滚动）收进 `Sink`，和 Logger 解耦；
- 不再有 `Raw`：需要原样输出的组件（如 GORM）直接持有 `Sink`。

## 快速开始

```go
logger, sink, err := log.New(log.Config{
	Level:    "info",
	Path:     "./logs/",   // 留空则只写 stdout，不落文件
	FileName: "app",
	Format:   log.FormatJSON,
	ShowFile: true,
})
if err != nil {
	return err
}
defer sink.Close()

logger.Info("hello", "key", "value")
logger.InfoContext(ctx, "request handled", "status", 200) // 自动带 request_id
```

`New` 返回的 `*slog.Logger` 是标准库类型，`ShowFile` 打开时 `source` 记录的就是调用点：

```json
{"time":"2026-11-20T10:26:54.421+08:00","level":"INFO","source":{"function":"main.main","file":"main.go","line":42},"msg":"hello","key":"value"}
```

## 输出格式

沿用原 `log` 包 slog 分支的配置（`slog.NewJSONHandler` / `slog.NewTextHandler` + `ReplaceAttr`），
级别、时间戳、调用点这些都与原来一致：

- `level` 大写：`DEBUG` / `INFO` / `WARN` / `ERROR`
- `time` 为 RFC3339Nano：`2026-11-20T10:26:54.421+08:00`
- `ShowFile` 打开时输出 `source` 分组：`function` / `file` / `line`
- `msg` 之后是结构化字段，`time.Duration` 输出为秒
- `Format: text` 为 slog TextHandler：`time=... level=INFO msg=... key=value`

## 为什么没有包装

如果包一层 `type Logger struct{ inner *slog.Logger }`，再让 `Info` 转发，
slog 内部固定深度的 `runtime.Callers(3, ...)` 会把来源记到包装层文件上。
不包装是让 `source` 行号正确的最简单方式，也省掉了 `WithContext` 存 ctx 这类怪设计。

需要接口做依赖注入时用 `log.Logger`（`*slog.Logger` 的方法子集）：

```go
var _ log.Logger = (*slog.Logger)(nil) // 编译期保证
```

## 配置

| 字段 | 默认 | 说明 |
|------|------|------|
| `Level` | `info` | `debug` / `info` / `warn` / `error`，兼容 `trace`/`verbose`/`notice`/`warning`/`fatal`，大小写与首尾空格不敏感 |
| `Path` | `""` | 空：只写 stdout；非空：同时写 `Path/FileName+FileSuffix`，自动滚动 |
| `FileName` | `runtime` | 不含后缀 |
| `FileSuffix` | `.logger` | |
| `MaxAge` | `30` | 保留天数；`-1` 表示不限 |
| `MaxBackups` | `0` | 最大备份数；`0` 不限 |
| `MaxSize` | `10` | 单文件上限（MB） |
| `Compress` | `false` | 归档是否压缩 |
| `ShowFile` | `false` | 是否记录调用点 |
| `Format` | `json` | `json` / `text` |

`time.Duration` 统一输出为秒（float64），避免 JSON 里出现纳秒整数。

## WithFields

map 形式的字段用 `log.WithFields` 展开（按 key 排序，输出稳定）：

```go
logger.With(log.WithFields(map[string]any{
	"order_id": "ORD-001",
	"amount":   99.9,
})...).InfoContext(ctx, "pay success")
```

`With` 本身就是标准库方法，`log.WithFields` 只是把 map 转成 `slog` 参数。

## Option

```go
log.New(cfg,
	log.WithAlerter(suppressor),
	log.WithFixedFields(map[string]any{"service": "api", "env": "prod"}),
	log.WithRedactKeys("password", "token", "secret", "authorization"),
	log.WithSampling(100, slog.LevelInfo), // info 及以下 1/100 抽样
)
```

| Option | 作用 |
|---|---|
| `WithAlerter(Alerter)` | 注入告警出口 |
| `WithFixedFields(map)` | 每条日志附加固定字段（按 key 排序） |
| `WithRedactKeys(...)` | 覆盖默认脱敏字段集合 |
| `WithRequestIDKeys(...)` | 从 ctx 注入哪些 key（默认 `request_id`），key 同时作为字段名 |
| `WithContextFields(...)` | 注入任意 ctx key（支持自定义类型），可指定字段名 |
| `WithSampling(every, below)` | 低于 `below` 级别的记录按 1/`every` 抽样 |

## 派生

```go
logger.With("module", "pay").WithGroup("db").Info("query", "sql", "select 1")
```

```json
{"level":"INFO","msg":"query","module":"pay","db":{"sql":"select 1"}}
```

`With` / `WithGroup` 都是标准库方法，返回 `*slog.Logger`。

## request_id

默认从 ctx 注入 `request_id`，已有同名字段则跳过：

```go
logger.InfoContext(ctx, "handled", "status", 200)
```

需要一次带多个链路字段时，在 `New` 时传一组 ctx key：

```go
logger, sink, _ := log.New(cfg,
	log.WithRequestIDKeys(defined.RequestID, defined.TransactionID, defined.RequestURI),
)

// ctx 里有什么就注入什么，取不到或为空的 key 直接跳过
logger.InfoContext(ctx, "handled") // request_id / transaction_id 都会带上
```

ctx key 是自定义类型时，用 `WithContextFields` 显式指定字段名：

```go
type traceKey struct{}

logger, sink, _ := log.New(cfg, log.WithContextFields(
	log.ContextField{Key: defined.RequestID, Name: "request_id"},
	log.ContextField{Key: traceKey{}, Name: "trace_id"},
))
```

key 同时是日志字段名；已经通过 `With` 显式传过的字段不会被覆盖。


## 告警

告警是标记，不是级别：`AlertKey`（`_alert`）为 true 的记录会转给 `Alerter`，
标记本身不会出现在输出里，且不参与采样。

```go
logger.With(log.AlertKey, true).Error("order.pay_failed", "trade_no", tradeNo)

type Alerter interface {
	Send(ctx context.Context, a Alert) error
}
```

`Alert` 带 `Title` / `Msg` / `Level` / `Time` / `Attrs`，`Fingerprint()` 用于去重
（`notify.Suppressor` 直接使用）。发送失败会记一条 `alert send failed`，不重试。

## 脱敏

默认把 `password` / `token` / `secret` / `authorization` 的值替换成 `***`，
分组内的同名字段同样处理，`With` 进去的字段也会被处理。

## GORM 等需要原样输出的场景

`Sink` 就是 `io.Writer`，直接拿它写原始文本：

```go
type GormLogWriter struct{ out io.Writer }

func (w *GormLogWriter) Printf(format string, v ...any) {
	line := strings.TrimRight(fmt.Sprintf(format, v...), "\r\n")
	_, _ = io.WriteString(w.out, line+"\n")
}
```

```go
logger, sink, _ := log.New(cfg)
gormLogger := gormlogger.New(NewGormLogWriter(sink), gormlogger.Config{...})
```

## 从旧版迁移

| 旧写法 | 新写法 |
|---|---|
| `l.WithField(k, v)` | `l.With(k, v)` |
| `l.WithFields(map)` | `l.With(k1, v1, k2, v2)` |
| `l.Raw(s)` | 用 `Sink` 直接写 |
| `logger.WithContext(ctx)` | `logger.InfoContext(ctx, ...)` |
| `log.WithContext(ctx)` / `log.Default()` | 已移除，用 `slog.Default()` / `slog.SetDefault` 或显式注入 |
| `NewLogger(cfg)` | `New(cfg)`，多一个 `Sink` 返回值 |
| `Config.Driver` | 已删除，只保留 slog |
