# log

基于标准库 `log/slog` 的日志包：**Logger 就是 `*slog.Logger`，不做任何包装**。

- 不管理配置：writer、level 都由调用方传给工厂；
- 只负责 Handler 组合：格式、ctx 字段、脱敏、告警、采样；
- 增强能力默认全关，用 `handler.Chain` 按需组合。

## 快速开始

```go
logger, err := log.New(log.JSON(os.Stdout, slog.LevelInfo))
if err != nil {
	return err
}

logger.Info("hello", "key", "value")
```

写文件 + 轮转用 `NewWriter`（零值即默认配置）：

```go
w := log.NewWriter(log.RotateConfig{Path: "./logs", FileName: "app"})
defer w.Close()

logger, err := log.New(log.JSON(w, slog.LevelInfo))
```

## 轮转 writer

```go
type RotateConfig struct {
	Path       string // 默认 ./logs/
	FileName   string // 默认 runtime
	FileSuffix string // 默认 .log
	MaxSize    int    // MB，默认 10
	MaxAge     int    // 天，默认 30；-1 不限
	MaxBackups int    // 0 不限
	Compress   bool
}

func NewWriter(cfg RotateConfig) RotateWriter
```

`RotateWriter` 内嵌 `io.Writer`，并实现 `io.Closer`：

```go
type RotateWriter struct {
	io.Writer
	// ...
}

func (w *RotateWriter) Close() error
```

行为：

- 固定写 `stdout + 文件`，用 `io.MultiWriter` 组合；
- `RotateConfig{}` 等价于 `./logs/runtime.log`、10MB 轮转、保留 30 天；
- 退出前调用 `Close()` 释放文件句柄。

## 内置工厂

| 工厂 | 输出 |
|---|---|
| `log.JSON(w, level)` | JSON |
| `log.Text(w, level)` | `key=value` 文本 |
| `log.Raw(w, level)` | 只写 msg + 换行 |
| `New()` 不传工厂 | `Raw(os.Stdout, Info)` |

`JSON` / `Text` 内置 `ReplaceAttr`：`Duration` 转秒、剥离 `_alert` 标记。

## Factory

```go
type Factory func() slog.Handler
```

writer 和级别都在闭包里捕获，`New` 只负责组合：

```go
log.New(log.JSON(os.Stdout, slog.LevelInfo))
log.New(log.JSON(file, slog.LevelInfo), log.Raw(os.Stderr, slog.LevelDebug))
```

- 多工厂用 `slog.NewMultiHandler` 组合；
- 工厂返回 nil 时 `New` 返回 error；
- 自定义工厂：

```go
log.New(func() slog.Handler {
	return slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{AddSource: true})
})
```

## 级别解析

```go
level := log.ParseLevel("warn") // 未知或空 → slog.LevelInfo
```

- 支持 `debug`/`trace`/`verbose`、`info`/`notice`、`warn`/`warning`、`error`/`err`/`fatal`；
- 不返回 error，未知一律回退 `info`。

## 组合能力

每个能力都是中间件，只收必要参数；`next` 由 `handler.Chain` 按数组顺序注入：

```go
h := handler.Chain(
	slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}),
	handler.NewSample(100, slog.LevelInfo),   // 最外层
	handler.NewContext(handler.ContextField{Key: defined.RequestID, Name: "request_id"}),
	handler.NewRedact("password", "token"),
	handler.NewAlert(alerter),                // 最内层
)

logger, err := log.New(func() slog.Handler { return h })
```

`Chain` 的语义：`mws[0]` 最外层，它的 `next` 是 `mws[1]`，依次到 `base`：

```text
Chain(base, A, B, C) == A(B(C(base)))
```

| 构造函数 | 必传参数 | 不满足时 |
|---|---|---|
| `handler.NewRaw(w, level)` | writer、级别 | — |
| `handler.NewContext(fields...)` | 至少一个字段 | identity |
| `handler.NewRedact(keys...)` | 至少一个 key | identity |
| `handler.NewAlert(alerter)` | alerter | identity |
| `handler.NewSample(every, below)` | — | `every <= 1` 时 identity |

推荐顺序（数组从外到内）：

```text
采样 -> ctx 字段 -> 脱敏 -> 告警 -> 格式
```

## 告警

告警是标记：`AlertKey`（`_alert`）为 true 的记录会转给 `Alerter`。

```go
logger.With(log.AlertKey, true).Error("order.pay_failed", "trade_no", tradeNo)
```

- `Alert` 带 `Title` / `Msg` / `Level` / `Time` / `Attrs`，`Fingerprint()` 用于去重；
- 标记不会出现在输出里（`NewAlert` 或内置 `JSON`/`Text` 的 `ReplaceAttr` 负责剥离）；
- 不组合 `NewAlert` 时不会发送。

## 和 GORM 的分工

GORM 的 SQL 日志是纯文本，由 `dao/gormDB` 自己持有 lumberjack 轮转，
不经过 slog；慢 SQL 告警由 GORM 的 `Sender` 直接发送。

## 从旧版迁移

| 旧写法 | 新写法 |
|---|---|
| `New(cfg)` | `New(factories...)` 或 `New(log.JSON(w, level))` |
| `Config.Path/FileName/Max*` | `log.NewWriter(log.RotateConfig{...})` |
| `Config.Level` | `log.ParseLevel(s)` 或直接 `slog.Level` |
| `New(cfg, opts...)` | `New(factories...)` |
| `WithAlerter` / `WithRedactKeys` / `WithContextFields` / `WithSampling` | `handler.Chain` + `NewAlert` / `NewRedact` / `NewContext` / `NewSample` |
| `Config.Format` / `ShowFile` | Factory 里的 `slog.HandlerOptions` |
| `log.Logger` 接口 | 直接用 `*slog.Logger` |
| `l.WithField(s)` | `l.With(...)` / `l.With(log.WithFields(m)...)` |
| `l.Raw(s)` | `log.Raw(w, level)` |
| `logger.WithContext(ctx)` | `logger.InfoContext(ctx, ...)` + `NewContext` |
