# HTTP 客户端库

这是一个功能强大的 HTTP 客户端库，支持多种 HTTP 客户端实现（FastHTTP、Resty），提供了统一的接口和丰富的功能。

## 特性

- 🚀 支持多种 HTTP 客户端（FastHTTP、Resty）
- 🔧 灵活的请求/响应处理器
- 🛡️ 内置请求验证和错误处理
- 📝 自动日志记录
- 🔐 应用认证支持
- ⏱️ 超时控制
- 🔄 响应解析和验证

## 快速开始

### 基本使用

```go
package main

import (
    "context"
    "github.com/Cotary/go-lib/net/http"
)

func main() {
    // 使用默认的 FastHTTP 客户端
    builder := http.NewRequestBuilder(http.DefaultFastHTTPClient)
    
    // 发送 GET 请求
    result := builder.Execute(
        context.Background(),
        "GET",
        "https://api.example.com/users",
        nil, nil, nil,
    )
    
    if result.Error != nil {
        panic(result.Error)
    }
    
    // 解析响应
    var users []User
    if err := result.Parse("data", &users); err != nil {
        panic(err)
    }
}
```

### 使用 Resty 客户端

```go
// 使用 Resty 客户端
builder := http.NewRequestBuilder(http.DefaultRestyClient)
builder.SetTimeout(30 * time.Second)

result := builder.Execute(
    context.Background(),
    "POST",
    "https://api.example.com/users",
    nil,
    map[string]interface{}{"name": "John", "email": "john@example.com"},
    map[string]string{"Content-Type": "application/json"},
)
```

### 添加请求处理器

```go
// 添加 URL 验证
builder.SetHandlers(
    http.URLValidationHandler(),
    http.RequestSizeLimitHandler(1024 * 1024), // 1MB 限制
    http.AuthAppHandler("app123", "secret456", "md5"),
)
```

### 添加响应处理器

```go
// 设置响应处理器
result.SetHandlers(
    http.StatusCodeCheckHandler(200, 201),
    http.JSONValidationHandler(),
    http.CodeCheckHandler(0, "code"), // 检查业务状态码
)
```

## 处理器说明

### 请求处理器 (RequestHandler)

- `URLValidationHandler()`: 验证 URL 格式
- `RequestSizeLimitHandler(maxSize)`: 限制请求体大小
- `AuthAppHandler(appID, secret, signType)`: 添加应用认证头

### 响应处理器 (ResponseHandler)

- `StatusCodeCheckHandler(codes...)`: 检查 HTTP 状态码
- `ResponseSizeCheckHandler(maxSize)`: 检查响应体大小
- `JSONValidationHandler()`: 验证 JSON 格式
- `CodeCheckHandler(code, field)`: 检查业务状态码
- `RetryOnErrorHandler(errors...)`: 标记可重试错误

## 配置选项

### 超时设置

```go
builder.SetTimeout(30 * time.Second)
```

### 日志控制

```go
builder.NoKeepLog()        // 不记录日志
builder.NoSendErrorMsg()   // 不发送错误消息
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

库提供了完善的错误处理机制：

```go
result := builder.Execute(ctx, "GET", "https://api.example.com/users", nil, nil, nil)

if result.Error != nil {
    // 检查是否为超时错误
    if fastClient.IsTimeout(result.Error) {
        // 处理超时
        log.Error("Request timeout")
        return
    }
    
    // 处理其他错误
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
2. **使用请求处理器**: 验证输入参数和添加必要的头信息
3. **使用响应处理器**: 验证响应格式和业务状态
4. **合理配置客户端**: 根据应用需求调整连接池和超时设置
5. **错误处理**: 正确处理各种错误情况
6. **日志记录**: 在生产环境中启用日志记录以便调试

## 注意事项

- FastHTTP 和 Resty 的超时处理方式不同
- 请求处理器按顺序执行，如果某个处理器失败，后续处理器不会执行
- 响应处理器在解析响应体之前执行
- 默认启用日志记录和错误消息发送，可以通过配置关闭
