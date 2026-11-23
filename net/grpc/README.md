# net/grpc

gRPC 客户端封装：连接管理、内置拦截器、错误还原。

## 快速开始

```go
c, err := grpcx.Dial(ctx, "order-service:9090",
    grpcx.WithInsecure(),
    grpcx.WithUnaryInterceptor(grpcx.RequestIDInterceptor()),
    grpcx.WithUnaryInterceptor(grpcx.AuthInterceptor(token)),
)
defer c.Close()

err = c.Invoke(ctx, "/order.v1.OrderService/GetOrder", req, reply)
```

**必须显式指定凭证**（`WithInsecure` / `WithTLS` / `WithCredentials` 三选一），都不写会返回 `no transport security set`。没有默认明文兜底 —— 忘了写凭证应该在启动时就报错，而不是静默把流量发成明文。

默认超时 30s，可用 `WithTimeout` 改。

## 错误处理

`Invoke` / `NewStream` 返回的是 **gRPC 原始错误，不做转换**。要业务语义自己取：

```go
err := c.Invoke(ctx, method, req, reply)

if codeErr, ok := grpcx.ToBusinessError(err); ok {
    // 业务错误：errors.Is 按业务码判定
    if errors.Is(codeErr, defined.OrderNotFound) { ... }
} else {
    // 不是业务错误：按 status code 判定
    if grpcx.Code(err) == codes.Unavailable { ... }
}
```

两种「尝试取值」的入口，取不到都返回 `nil, false`，不拿默认值糊弄：

| 函数 | 取什么 |
|---|---|
| `ToBusinessError(err) (*e.CodeErr, bool)` | 业务语义：`Code` / `Class` / `Msg` |
| `ToGRPCStatus(err) (*status.Status, bool)` | 传输语义：`code` / `message` / `details` |

### 还原规则

| 线上字段 | 还原成 |
|---|---|
| `status.code` | `e.Class`（类别，0~16） |
| `ErrorInfo.metadata.biz_code` | `e.Code`（业务码） |
| `status.message` | `e.Msg`（文案） |

**对端没带 `biz_code` 时返回 `nil, false`** —— 不构造业务错误，也不拿协议状态码冒充业务码（1~16 会和真实业务码撞车）。

### 便捷读取

```go
grpcx.Code(err)             // codes.Code，非 status 返回 Unknown
grpcx.IsStatus(err)         // 是否 gRPC status 错误
grpcx.IsRetryable(err)      // 是否值得重试
grpcx.IsNotFound(err)       // 是否 NotFound
grpcx.IsUnauthenticated(err)// 是否需要认证
grpcx.Reason(err)           // ErrorInfo.reason（对端写入的业务文案）
grpcx.BizCode(err)          // 业务码，如 20001
grpcx.Domain(err)           // 服务名
grpcx.Details(err)          // 原始 details 数组
grpcx.FieldViolations(err)  // []e.FieldViolation，字段级校验错误
```

## 内置拦截器

| 拦截器 | 作用 | stream 版 |
|---|---|---|
| `RequestIDInterceptor()` | 生成/透传 `x-request-id` 到 outgoing metadata | `RequestIDStreamInterceptor()` |
| `AuthInterceptor(token, key...)` | 注入凭证，默认 `authorization: Bearer <token>` | — |
| `LogInterceptor(logger)` | method、code、耗时、target | — |
| `RetryInterceptor(policy...)` | 按状态码白名单重试 | — |

`AuthInterceptor` 的 `key` 可换 metadata key（如 `x-token`），换后不加 `Bearer` 前缀。

## 重试

`RetryInterceptor` 按 **gRPC 状态码白名单**判断：

| 状态码 | 重试 | 理由 |
|---|---|---|
| `Unavailable` | ✅ | 服务暂时不可用，退避后可能成功 |
| `ResourceExhausted` | ✅ | 配额/限流，退避后可能成功 |
| `Aborted` | ✅ | 并发冲突，重试可能成功 |
| 其余 | ❌ | 重试多半还是错 |

```go
grpcx.RetryInterceptor(grpcx.RetryPolicy{
    MaxRetries: 2,
    Backoff:    100 * time.Millisecond,
    Multiplier: 2,
    MaxBackoff: 2 * time.Second,
})
```

⚠️ **只对幂等操作挂。** 非幂等的写操作重试可能写坏数据。

## 流式调用

```go
stream, err := c.NewStream(ctx, &grpc.StreamDesc{
    ServerStreams: true,
}, "/order.v1.OrderService/WatchOrders")
if err != nil {
    return err
}
defer stream.Close()

for {
    msg, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        return err
    }
    // ...
}
```

`NewStream` 返回的 `ClientStream` 带 `Close()`，会释放 `WithTimeout` 挂上的超时。

## 选项

| 函数 | 作用 |
|---|---|
| `WithInsecure()` | 明文连接 |
| `WithTLS(cfg)` / `WithCredentials(creds)` | TLS / 自定义凭证 |
| `WithPerRPCCredentials(creds)` | 每次调用的凭证 |
| `WithUnaryInterceptor(i)` / `WithStreamInterceptor(i)` | 追加拦截器 |
| `WithTimeout(d)` | 每次调用默认超时，默认 30s |
| `WithKeepalive(params)` | keepalive |
| `WithUserAgent(ua)` | User-Agent |
| `WithDialOption(opts...)` | 透传原生 `grpc.DialOption` |

`WithDialOption` 用于本库没封装的能力：

```go
grpcx.WithDialOption(grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
grpcx.WithDialOption(grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
```

⚠️ 传 `grpc.WithTransportCredentials` 会和 `WithInsecure` / `WithTLS` 冲突，以后生效的为准，不报错。
同样不要用它传 `grpc.WithChainUnaryInterceptor`，会和 `WithUnaryInterceptor` 形成两条链。

## 其它

| 方法 | 说明 |
|---|---|
| `Conn()` | 拿原生 `*grpc.ClientConn` |
| `Close()` | 关闭连接 |
| `Ping(ctx)` | 触发一次连接尝试（grpc 连接是惰性的）。**不等待就绪，返回值恒为 nil** |

⚠️ `Auth` / `Log` / `Retry` 三个拦截器**只有 unary 版**。流式调用只自动带 `RequestIDStreamInterceptor`，需要凭证的流式接口要自己塞 metadata。

⚠️ **升级提示**：`Dial` 不再有「未指定凭证时默认明文」的兜底。
以前只传拦截器（不传凭证）的调用方，会在升级后收到 `no transport security set`，需要补一个 `WithInsecure()` 或 `WithTLS()`。
