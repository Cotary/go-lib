# httptransport - nodepool HTTP 传输层

为 `provider/nodepool` 提供开箱即用的 HTTP Transport 实现，基于 `net/http` 的 FastHTTP 客户端和洋葱模型中间件体系。

## 功能特性

- **URL 标准化拼接** — endpoint（baseURL）+ path 自动拼接，无需手动处理斜杠
- **三级 Header 合并** — 全局默认 → 节点级 → 请求级，优先级递增
- **底层复用 FastHTTP** — 默认使用高性能 FastHTTP 客户端，也可替换为 Resty 等
- **灵活的日志控制** — 内置请求日志，可全局关闭或通过中间件自定义
- **中间件集成** — 直接复用 `net/http` 的认证、签名、重试、计时等中间件
- **HTTP 状态码分类器** — 自动将 HTTP 响应映射为 nodepool 的 Success/Fail/BizError

## 快速开始

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/Cotary/go-lib/provider/nodepool"
    "github.com/Cotary/go-lib/provider/nodepool/httptransport"
)

func main() {
    // 1. 创建 HTTP Transport
    transport := httptransport.New(
        httptransport.WithDefaultHeaders(map[string]string{
            "Content-Type":  "application/json",
            "Authorization": "Bearer your-api-key",
        }),
        httptransport.WithTimeout(10 * time.Second),
    )

    // 2. 创建响应分类器
    classifier := httptransport.NewClassifier()

    // 3. 创建节点池
    pool, err := nodepool.New(transport, classifier, []nodepool.NodeConfig{
        {Endpoint: "https://api1.example.com"},
        {Endpoint: "https://api2.example.com"},
    })
    if err != nil {
        panic(err)
    }
    defer pool.Close()

    // 4. 发起请求
    resp, err := pool.Do(context.Background(), &nodepool.Request{
        Data: &httptransport.HTTPRequest{
            Method: http.MethodPost,
            Path:   "/v1/chat/completions",
            Body:   map[string]any{"model": "gpt-4", "prompt": "hello"},
        },
    })
    if err != nil {
        panic(err)
    }

    // 5. 解析响应
    httpResp := resp.Data.(*httptransport.HTTPResponse)
    fmt.Printf("状态码: %d, 响应: %s\n", httpResp.StatusCode, httpResp.String())
}
```

## 架构概览

```
                    nodepool.Pool
                        │
                  Pool.Do(req)
                        │
              ┌─────────▼──────────┐
              │  httptransport     │
              │  .Transport        │
              │                    │
              │  1. 提取 HTTPRequest│
              │  2. 拼接 URL       │
              │  3. 合并 Header    │
              │  4. 构建 Builder   │
              │  5. 执行中间件链   │
              └─────────┬──────────┘
                        │
              ┌─────────▼──────────┐
              │  net/http          │
              │  RequestBuilder    │
              │  + Middleware Chain │
              └─────────┬──────────┘
                        │
              ┌─────────▼──────────┐
              │  FastHTTPClient    │
              │  (valyala/fasthttp)│
              └────────────────────┘
```

## 配置选项

### Transport 选项

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `WithClient(client)` | 自定义 HTTP 客户端实现 | `FastHTTPClient` |
| `WithDefaultHeaders(h)` | 全局默认 Header | 空 |
| `WithNodeHeaders(endpoint, h)` | 节点级 Header | 空 |
| `WithMiddleware(m...)` | 添加中间件 | 无 |
| `WithKeepLog(bool)` | 是否记录请求日志 | `true` |
| `WithSendErrorMsg(bool)` | 出错时是否发送通知 | `true` |
| `WithTimeout(d)` | 默认请求超时 | 0（使用客户端默认） |

### Classifier 选项

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `WithFailCodes(codes...)` | 标记为节点故障的状态码 | `429` |
| `WithBizErrCodes(codes...)` | 标记为业务错误的状态码 | 无 |
| `WithCustomClassify(fn)` | 自定义分类函数 | 无 |

### 默认分类规则

| 状态码范围 | 分类 | 说明 |
|-----------|------|------|
| 2xx / 3xx | `NodeStatusSuccess` | 请求成功 |
| 429 | `NodeStatusFail` | 限流，触发重试和健康度扣分 |
| 5xx | `NodeStatusFail` | 服务端错误，触发重试 |
| 其他 4xx | `NodeStatusBizError` | 业务错误，不重试 |

## URL 拼接规则

- `endpoint`（来自 NodeConfig）作为 baseURL
- `HTTPRequest.Path` 作为路径
- 自动处理首尾斜杠：`https://api.com/` + `/v1/users` → `https://api.com/v1/users`
- 如果 Path 以 `http://` 或 `https://` 开头，直接作为完整 URL
- Path 为空时直接使用 endpoint

## Header 合并优先级

```
优先级从低到高:

1. WithDefaultHeaders()     -- 全局默认
2. WithNodeHeaders()        -- 按节点设置
3. HTTPRequest.Headers      -- 单次请求
```

高优先级的同名 Header 会覆盖低优先级。

## 高级用法

### 不同节点使用不同 API Key

```go
transport := httptransport.New(
    httptransport.WithNodeHeaders("https://openai-api.com", map[string]string{
        "Authorization": "Bearer sk-openai-xxx",
    }),
    httptransport.WithNodeHeaders("https://azure-api.com", map[string]string{
        "api-key": "azure-key-xxx",
    }),
)
```

### 关闭日志

```go
transport := httptransport.New(
    httptransport.WithKeepLog(false),
    httptransport.WithSendErrorMsg(false),
)
```

### 添加认证中间件

```go
import nethttp "github.com/Cotary/go-lib/net/http"

transport := httptransport.New(
    httptransport.WithMiddleware(
        nethttp.AuthMiddleware("appId", "secret", "md5"),
        nethttp.TimingMiddleware(),
    ),
)
```

### 自定义分类器（按业务码判断）

```go
classifier := httptransport.NewClassifier(
    httptransport.WithCustomClassify(func(statusCode int, body []byte, err error) nodepool.NodeStatus {
        if err != nil {
            return nodepool.NodeStatusFail
        }
        if statusCode >= 500 || statusCode == 429 {
            return nodepool.NodeStatusFail
        }
        // 解析业务码
        code := gjson.GetBytes(body, "code").Int()
        if code != 0 {
            return nodepool.NodeStatusBizError
        }
        return nodepool.NodeStatusSuccess
    }),
)
```

## 技术原理

### 与 nodepool 的集成

`httptransport.Transport` 实现了 `nodepool.Transport` 接口：

- `nodepool.Request.Data` 存放 `*HTTPRequest`（方法、路径、请求体等）
- `nodepool.Response.Data` 存放 `*HTTPResponse`（状态码、响应头、响应体）
- nodepool 本身保持协议无关，所有 HTTP 细节封装在此包中

### 中间件复用

直接复用 `net/http` 包的洋葱模型中间件链，不重复造轮子：

- 内置日志（`RequestLogMiddleware`）自动记录请求/响应
- 可挂载认证（`AuthMiddleware`）、重试（`RetryMiddleware`）、计时（`TimingMiddleware`）等
- 支持 `ConditionalMiddleware` 按条件执行
