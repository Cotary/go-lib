# log

基于 Go 标准库 `log/slog` 的结构化日志封装，支持 `zap` 和 `slog` 两种后端驱动，内置 `lumberjack` 文件滚动。

## 架构

```
业务代码 → log.WithContext(ctx) / logger.Info(...)
         → SlogWrapper (统一封装)
         → slog.Handler (ZapHandler 或 slog 原生 Handler)
         → io.MultiWriter → os.Stdout + lumberjack (文件滚动)
```

## 快速开始

### 最小配置

```go
import "github.com/Cotary/go-lib/log"

// 全部使用默认值：zap 驱动, debug 级别, JSON 格式, 输出到 ./logs/runtime.logger
logger := log.NewLogger(&log.Config{})
defer logger.Close()

logger.Info("hello", "key", "value")
```

### 全局初始化（推荐）

```go
import (
    lib "github.com/Cotary/go-lib"
    "github.com/Cotary/go-lib/log"
)

func main() {
    logger := log.NewLogger(&log.Config{
        Driver:   log.DriverZap,
        Level:    "info",
        Path:     "./logs/",
        FileName: "app",
        Format:   log.FormatJSON,
        ShowFile: true,
    })
    defer logger.Close()

    lib.InitLog(logger)

    // 之后任意位置使用：
    log.WithContext(ctx).Info("request handled", "status", 200)
}
```

## 配置说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| Driver | string | `"zap"` | 驱动：`zap`、`slog` |
| Level | string | `"debug"` | 级别：`debug`、`info`、`warn`、`error` |
| Path | string | `"./logs/"` | 日志文件目录 |
| FileName | string | `"runtime"` | 文件名（不含后缀） |
| FileSuffix | string | `".logger"` | 文件后缀 |
| MaxAge | int64 | `30` | 文件保留天数（设 -1 表示不限） |
| MaxBackups | int64 | `0` | 最大备份文件数（0 = 不限） |
| MaxSize | int64 | `10` | 单文件大小上限（MB） |
| Compress | bool | `false` | 是否压缩归档文件 |
| ShowFile | bool | `false` | 是否显示调用位置（文件名:行号） |
| Format | string | `"json"` | 输出格式：`json`、`text` |

## 使用方式

### 带 Context 的请求级日志

最常用方式。自动从 `context` 提取 `request_id` 并注入日志字段：

```go
func HandleRequest(ctx context.Context) {
    log.WithContext(ctx).Info("processing request",
        "user_id", 123,
        "action", "login",
    )
}
```

### WithField / WithFields 链式调用

```go
logger := log.WithContext(ctx)

// 单字段
logger.WithField("module", "payment").Info("charge created")

// 多字段
logger.WithFields(map[string]any{
    "order_id": "ORD-001",
    "amount":   99.9,
}).Info("payment success")

// 链式
logger.WithField("a", 1).WithField("b", 2).Info("chained")
```

### Duration 自动转秒

所有驱动统一将 `time.Duration` 类型的值转为秒（float64）：

```go
cost := 1234 * time.Millisecond
logger.Info("query done", "cost", cost)
// 输出: "cost": 1.234
```

### Raw 原始输出

直接写入原始字符串，不带时间戳、级别等格式化信息。适用于 GORM 等需要自定义格式的场景：

```go
logger.Raw(`{"sql":"SELECT * FROM users","rows":10}`)
```

> **注意**：`Raw` 不受级别过滤控制，即使 Level 设为 error，Raw 仍会输出。

### 独立实例

不使用全局 logger，创建独立实例（如给 GORM 单独配置）：

```go
dbLogger := log.NewLogger(&log.Config{
    Driver:   log.DriverZap,
    Level:    "warn",
    Path:     "./logs/",
    FileName: "db",
    Format:   log.FormatText,
})
defer dbLogger.Close()
```

## 驱动选择

| 驱动 | 特点 | 适用场景 |
|------|------|----------|
| `zap` (默认) | 高性能、零分配、JSON/Console 编码 | 生产环境、高吞吐服务 |
| `slog` | Go 标准库、零外部依赖 | 轻量项目、减少依赖 |

## 注意事项

- **程序退出前调用 `Close()`**：确保 lumberjack 文件 writer 正确关闭，避免日志丢失。
- **`Close()` 仅对 `NewLogger` 创建的 root logger 有效**：通过 `WithField` / `WithContext` 派生的 logger 无需调用。
- **`WithContext` 自动初始化**：如果未调用 `SetGlobalLogger`，首次使用 `WithContext` 会以默认配置自动初始化全局 logger。
- **`NewLogger` 不会修改传入的 Config**：内部会拷贝一份再填充默认值。
- **`slog.Handler.WithGroup` 未实现**：当前 ZapHandler 的 `WithGroup` 调用无效果（静默忽略）。正常使用中不受影响。
