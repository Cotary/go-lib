# HTTP 客户端库

基于洋葱模型的 HTTP 客户端库，支持多种 HTTP 客户端实现（FastHTTP、Resty），提供统一的接口和丰富的中间件功能。

## 特性

- 🧅 **洋葱模型中间件** - 统一的前置/后置处理
- 🚀 支持多种 HTTP 客户端（FastHTTP、Resty）
- 🛡️ 内置请求验证和错误处理
- 📝 自动日志记录
- 🔐 应用认证支持
- ⏱️ 超时控制
- 🔄 响应解析和验证

## 快速开始

### 基本使用（推荐）

```go
package main

import (
    "context"
    "go-lib/net/http"
)

func main() {
    // 推荐：使用 FastHTTP 快捷函数（性能更优）
    result := http.FastHTTP().Execute(
        context.Background(),
        "GET",
        "https://api.example.com/users",
        nil, nil, nil,
    )
    
    if result.HasError() {
        panic(result.Error)
    }
    
    // 方式一：使用泛型工具函数解析响应
    data, err := http.Parse[map[string]interface{}](result, "data")
    if err != nil {
        panic(err)
    }
    
    // 方式二：链式调用 ParseTo
    var user User
    err = result.ParseTo("data.user", &user)
}
```

### 使用 Resty 客户端

```go
// 使用 Resty 快捷函数
result := http.RestyHTTP().SetTimeout(30 * time.Second).Execute(
    context.Background(),
    "POST",
    "https://api.example.com/users",
    nil,
    map[string]interface{}{"name": "John", "email": "john@example.com"},
    map[string]string{"Content-Type": "application/json"},
)

// 方式一：使用泛型工具函数解析
user, err := http.Parse[User](result, "data")

// 方式二：链式调用 ParseTo
var user2 User
err = result.ParseTo("data", &user2)
```

## 洋葱模型中间件

洋葱模型是一种优雅的中间件模式，每个中间件可以同时处理请求前和请求后的逻辑。

### 执行流程

```
请求 → [中间件1前] → [中间件2前] → HTTP请求 → [中间件2后] → [中间件1后] → 响应
        ↓                ↓                     ↑              ↑
        └─────────────── next() ───────────────┘              │
                                                              │
        └──────────────────────────────────────────────────────┘
```

### 添加中间件

```go
builder := http.NewRequestBuilder(http.DefaultFastHTTPClient)

// 添加中间件（按注册顺序执行）
builder.Use(
    http.RecoveryMiddleware(),           // 最外层：捕获 panic
    http.TimingMiddleware(),             // 记录请求耗时
    http.LoggingMiddleware(logger),      // 日志记录
    http.TracingMiddleware(),            // 追踪 ID
    http.AuthMiddleware("app", "secret", "md5"), // 认证
    http.StatusCodeCheckMiddleware(200), // 状态码检查
)

result := builder.Execute(ctx, "GET", url, nil, nil, nil)

// 需要解析时使用泛型工具函数
data, err := http.Parse[MyResponse](result, "")
```

### 自定义中间件

```go
// 自定义中间件示例：请求/响应审计（Gin 风格）
func AuditMiddleware(auditLog log.Logger) http.Middleware {
    return func(ctx *http.Context) {
        // ========== 请求前逻辑 ==========
        startTime := time.Now()
        auditLog.Info("Request started", 
            "method", ctx.Request.Method,
            "url", ctx.Request.URL,
        )
        
        // 执行下一个中间件
        ctx.Next()
        
        // ========== 请求后逻辑 ==========
        duration := time.Since(startTime)
        if ctx.Error != nil {
            auditLog.Error("Request failed",
                "duration", duration,
                "error", ctx.Error,
            )
        } else {
            auditLog.Info("Request completed",
                "duration", duration,
                "status", ctx.Response.StatusCode,
            )
        }
    }
}
```

### 内置中间件

| 中间件 | 说明 | 阶段 |
|--------|------|------|
| `RequestLogMiddleware()` | **内置**：自动记录请求/响应日志（由 Builder 自动添加） | 后 |
| `RecoveryMiddleware()` | 捕获 panic，防止程序崩溃 | 前+后 |
| `TimingMiddleware()` | 记录请求耗时 | 前+后 |
| `LoggingMiddleware(logger)` | 自定义 Logger 的请求/响应日志 | 前+后 |
| `TracingMiddleware()` | 添加追踪 ID | 前+后 |
| `RetryMiddleware(maxRetries, delay)` | 自动重试 | 包装 |
| `AuthMiddleware(appID, secret, signType)` | 应用认证 | 前 |
| `URLValidationMiddleware()` | URL 格式验证 | 前 |
| `RequestSizeLimitMiddleware(maxSize)` | 请求体大小限制 | 前 |
| `HeaderMiddleware(headers)` | 添加自定义请求头 | 前 |
| `TimeoutMiddleware(timeout)` | 设置超时 | 前 |
| `StatusCodeCheckMiddleware(codes...)` | HTTP 状态码检查 | 后 |
| `CodeCheckMiddleware(code, field)` | 业务状态码检查 | 后 |
| `JSONValidationMiddleware()` | JSON 格式验证 | 后 |
| `ResponseSizeCheckMiddleware(maxSize)` | 响应体大小检查 | 后 |

> **注意**：`RequestLogMiddleware` 由 `RequestBuilder` 自动添加在中间件链最外层，通常不需要手动添加。

### 中间件组合

```go
// 组合多个中间件为一个
authMiddleware := http.Compose(
    http.TracingMiddleware(),
    http.AuthMiddleware("app", "secret", "md5"),
    http.HeaderMiddleware(map[string]string{
        "X-Client-Version": "1.0.0",
    }),
)

builder.Use(authMiddleware)
```

### 条件中间件

```go
// 只在生产环境启用日志
builder.Use(
    http.ConditionalMiddleware(
        func(ctx *http.Context) bool {
            return os.Getenv("ENV") == "production"
        },
        http.LoggingMiddleware(logger),
    ),
)
```

### 中间件上下文传递数据

```go
func CustomMiddleware() http.Middleware {
    return func(ctx *http.Context) {
        // 设置值
        ctx.Set("start_time", time.Now())
        
        ctx.Next()
        
        // 获取值
        if startTime, ok := ctx.Get("start_time"); ok {
            duration := time.Since(startTime.(time.Time))
            fmt.Println("Duration:", duration)
        }
    }
}
```

### 中断中间件链

```go
func AuthCheckMiddleware() http.Middleware {
    return func(ctx *http.Context) {
        token := ctx.Request.Headers["Authorization"]
        if token == "" {
            ctx.AbortWithError(errors.New("unauthorized"))
            return  // 不调用 ctx.Next()，后续中间件不会执行
        }
        ctx.Next()
    }
}
```

### 推荐的中间件顺序

```go
builder.Use(
    // 1. 最外层：错误恢复
    http.RecoveryMiddleware(),
    
    // 2. 计时和追踪
    http.TimingMiddleware(),
    http.TracingMiddleware(),
    
    // 3. 日志（需要在认证之前，以便记录完整信息）
    http.LoggingMiddleware(logger),
    
    // 4. 请求预处理
    http.URLValidationMiddleware(),
    http.RequestSizeLimitMiddleware(10*1024*1024),
    
    // 5. 认证
    http.AuthMiddleware("app", "secret", "md5"),
    
    // 6. 响应后处理
    http.StatusCodeCheckMiddleware(200, 201),
    http.JSONValidationMiddleware(),
)
```

## 配置选项

### 超时设置

```go
builder.SetTimeout(30 * time.Second)
```

### 日志控制

日志功能由内置的 `RequestLogMiddleware` 中间件实现，默认启用。

```go
builder.NoKeepLog()        // 禁用自动日志记录
builder.NoSendErrorMsg()   // 禁用错误消息发送

// 手动记录日志（在 NoKeepLog 模式下）
result := builder.NoKeepLog().Execute(ctx, "GET", url, nil, nil, nil)
result.Log(logger)  // 手动调用
```

### 自定义客户端

```go
// 自定义 FastHTTP 客户端
customFastClient := &fasthttp.Client{
    MaxConnsPerHost: 1000,
    ReadTimeout:     30 * time.Second,
    WriteTimeout:    30 * time.Second,
}
builder := http.NewRequestBuilder(http.NewFastHTTPClient(customFastClient))

// 自定义 Resty 客户端
customRestyClient := resty.New()
customRestyClient.SetTimeout(30 * time.Second)
builder := http.NewRequestBuilder(http.NewRestyClient(customRestyClient))
```

## 错误处理

```go
result := builder.Execute(ctx, "GET", "https://api.example.com/users", nil, nil, nil)

// 检查错误
if result.HasError() {
    // 检查是否为超时错误
    if fastClient.IsTimeout(result.Error) {
        log.Error("Request timeout")
        return
    }
    
    log.Error("Request failed:", result.Error)
    return
}

// 检查响应状态
if result.Response.StatusCode >= 400 {
    log.Error("HTTP error:", result.Response.StatusCode)
    return
}
```

## 最佳实践

1. **总是设置超时**: 避免请求无限期等待
2. **使用中间件**: 验证输入参数和添加必要的头信息
3. **合理配置客户端**: 根据应用需求调整连接池和超时设置
4. **错误处理**: 正确处理各种错误情况
5. **日志记录**: 在生产环境中启用日志记录以便调试
6. **中间件顺序**: 按推荐顺序配置中间件

## 注意事项

- FastHTTP 和 Resty 的超时处理方式不同（详见下方对比）
- 中间件按注册顺序执行，请求时从外到内，响应时从内到外
- 默认启用日志记录和错误消息发送，可以通过配置关闭
- 如果某个中间件返回错误，后续中间件的"前置"逻辑不会执行，但已执行中间件的"后置"逻辑仍会执行

## FastHTTP vs Resty 选择指南

两个客户端都实现了 `IClient` 接口，API 完全一致，但底层行为有差异：

|  | FastHTTP | Resty |
|--|----------|-------|
| **性能** | 高（零分配优化、连接池复用） | 中（基于标准库 net/http） |
| **适用场景** | 高并发、大量短连接、对延迟敏感 | 通用场景、需要完整 HTTP 特性 |
| **HTTP/2** | 不支持 | 支持（标准库内置） |
| **Context 取消** | 支持（通过 goroutine 监听实现） | 原生支持 |
| **超时机制** | `fasthttp.Request.SetTimeout` + context deadline | `context.WithTimeout` |
| **默认连接池** | `MaxConnsPerHost: 1000` | 标准库默认值 |
| **Cookie 管理** | 需手动处理 | 内置支持 |
| **重定向** | 需手动处理 | 内置支持（可配置） |

### 推荐选择

- **需要极致性能**（如请求池、大量并发 RPC 调用）：使用 `FastHTTP()`
- **通用业务请求**（如调用第三方 API、需要 HTTP/2）：使用 `RestyHTTP()`
- **不确定时**：优先使用 `FastHTTP()`，性能更好且功能已覆盖大部分场景

### Context 取消行为

两个客户端均支持通过 `context.Context` 取消请求：

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// 两种客户端都会在 context 取消时中断请求
result := http.FastHTTP().Execute(ctx, "GET", url, nil, nil, nil)
result := http.RestyHTTP().Execute(ctx, "GET", url, nil, nil, nil)
```

**实现差异**：
- **Resty**: 原生通过 `http.Request.WithContext` 实现，标准库直接响应取消
- **FastHTTP**: 通过独立 goroutine 监听 `ctx.Done()`，取消时关闭连接中断请求。极端情况下响应速度略慢于 Resty

两种客户端的 `IsTimeout()` 方法都能正确识别 context 超时错误。
