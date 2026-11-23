# net/grpc

gRPC 客户端封装。错误处理与服务端同源：服务端用 `google.rpc.ErrorInfo` 承载业务码和文案，客户端把它还原成业务错误。

## 用法

```go
c, err := grpcx.Dial(ctx, "order-service:9090",
    grpcx.WithInsecure(),
    grpcx.WithUnaryInterceptor(grpcx.RequestIDInterceptor()),
    grpcx.WithUnaryInterceptor(grpcx.AuthInterceptor(token)),
    grpcx.WithUnaryInterceptor(grpcx.RetryInterceptor()),
)
defer c.Close()

err := c.Invoke(ctx, "/order.v1.OrderService/GetOrder", req, reply)
```

`Invoke` 返回的错误**已经过还原**，可以直接用 `errors.Is` 判断业务语义：

```go
if errors.Is(err, defined.OrderNotFound) {
    // 订单不存在
}
```

## 错误还原

服务端返回的是 `google.rpc.Status`，客户端按下面的规则还原：

| 来源 | 还原成 |
|---|---|
| `status.code` | `e.Class`（类别） |
| `ErrorInfo.metadata.biz_code` | `e.Code`（业务码） |
| `status.message` | `e.Msg`（文案） |
| `ErrorInfo.reason` | 业务文案，仅用于排查，**不参与类别判定** |
| `ErrorInfo.domain` | 服务名 |

`errors.Is` 按**业务码**判定，所以同一类别下的不同错误不会互相误判。

⚠️ **对端没带 `biz_code` 时**（第三方服务、其他语言实现），`ToBusinessError` 返回 `nil, false` —— 不构造业务错误，也不会拿协议状态码冒充业务码。

```go
// ToBusinessError 只取出业务三元组（Code / Class / Msg）
if codeErr, ok := grpcx.ToBusinessError(rawErr); ok {
    errors.Is(codeErr, defined.OrderNotFound)   // 按业务码判定
} else {
    // 不是业务错误（纯协议错误或对端没带业务码），按 status code 判断
    grpcx.Code(rawErr)
}
```

`Client.Invoke` 返回的是 **gRPC 原始错误，不做任何转换**。需要业务语义就自己调一次：

```go
err := client.Invoke(...)
if codeErr, ok := grpcx.ToBusinessError(err); ok {
    errors.Is(codeErr, defined.OrderNotFound)
} else {
    grpcx.Code(err)   // 不是业务错误，按 status code 判断
}
```

```go
// 都是 NOT_FOUND，但业务码不同，判定互不干扰
errors.Is(err, defined.OrderNotFound)   // true
errors.Is(err, defined.UserNotFound)    // false
```

### 辅助函数

两种「尝试取值」的入口，取不到都返回 `nil, false`，不拿默认值糊弄：

```go
codeErr, ok := grpcx.ToBusinessError(err)  // 业务语义：Code / Class / Msg
st, ok := grpcx.ToGRPCStatus(err)          // 传输语义：code / message / details
```

基于 `ToGRPCStatus` 的便捷读取：

```go
grpcx.Code(err)               // codes.Code（非 status 返回 Unknown）
grpcx.IsStatus(err)           // 是否 gRPC status 错误
grpcx.Reason(err)             // 类别常量，如 "NOT_FOUND"
grpcx.BizCode(err)            // 业务码，如 20001
grpcx.Domain(err)             // 服务名
grpcx.FieldViolations(err)    // []e.FieldViolation，字段级校验错误
grpcx.Details(err)            // 原始 details 数组
```

## 重试语义

`RetryInterceptor` 按 **gRPC 状态码白名单**判断，而不是 HTTP 的 5xx：

| 状态码 | 重试 | 理由 |
|---|---|---|
| `Unavailable` | ✅ | 服务暂时不可用，退避后可能成功 |
| `ResourceExhausted` | ✅ | 配额/限流，退避后可能成功 |
| `Aborted` | ✅ | 并发冲突，重试可能成功 |
| `Internal` | ❌ | 重试多半还是错 |
| `InvalidArgument` | ❌ | 重试还是错 |
| `DeadlineExceeded` | ❌ | 默认不重试，更安全 |

**这是 gRPC 相对 HTTP 的实打实收益。** HTTP 那边所有错误都是 200 + body code，客户端根本无从判断「该不该重试」；gRPC 的状态码自带这个语义。

⚠️ **只对幂等操作挂这个拦截器。** 非幂等的写操作重试可能写坏数据。

```go
grpcx.RetryInterceptor(grpcx.RetryPolicy{
    MaxRetries: 2,
    Backoff:    100 * time.Millisecond,
    Multiplier: 2,
    MaxBackoff: 2 * time.Second,
})
```

## 流式调用

```go
stream, err := c.NewStream(ctx, &grpc.StreamDesc{
    ServerStreams: true,
    ClientStreams: true,
}, "/order.v1.OrderService/WatchOrders")
if err != nil {
    return err
}
defer stream.Close()

stream.Send(req)
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

## 客户端拦截器

| 拦截器 | 作用 |
|---|---|
| `RequestIDInterceptor()` | 生成/透传 `x-request-id` 到 outgoing metadata |
| `AuthInterceptor(token)` | 凭证注入 outgoing metadata |
| `LogInterceptor(logger)` | method、code、耗时 |
| `RetryInterceptor(policy...)` | 按状态码白名单重试 |

## 其它

```go
grpcx.Code(err)              // gRPC 状态码
grpcx.IsRetryable(err)       // 是否值得重试
grpcx.IsNotFound(err)        // 是否 NotFound
grpcx.IsUnauthenticated(err) // 是否需要认证
```
