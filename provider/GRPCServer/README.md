# provider/GRPCServer

gRPC 服务端封装：生命周期、健康检查、反射、错误收口、内置拦截器。

业务层只声明错误语义，映射成 `google.rpc.Status` 的工作统一由 `ErrorUnaryInterceptor` 完成。

## 快速开始

```go
srv := grpcserver.New(grpcserver.Config{Addr: ":9090"},
    grpcserver.WithUnaryInterceptor(grpcserver.RecoveryUnaryInterceptor()),
    grpcserver.WithUnaryInterceptor(grpcserver.RequestIDInterceptor()),
    grpcserver.WithUnaryInterceptor(grpcserver.LogUnaryInterceptor(nil)),
    grpcserver.WithUnaryInterceptor(grpcserver.ErrorUnaryInterceptor()),   // ★ 必须最后
    grpcserver.WithStreamInterceptor(grpcserver.RecoveryStreamInterceptor()),
    grpcserver.WithStreamInterceptor(grpcserver.ErrorStreamInterceptor()), // ★ 必须最后
)

srv.RegisterHealthService()   // grpc.health.v1，K8s 探针直接用
srv.RegisterReflection()      // grpcurl 自检

pb.RegisterOrderServiceServer(srv.GRPC(), impl)
go func() { _ = srv.ListenAndServe(ctx) }()
```

业务实现直接返回 error，不需要自己转 status：

```go
func (s *impl) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.GetOrderReply, error) {
    order, err := s.usecase.Get(ctx, req.Id)
    if err != nil {
        return nil, err
    }
    return &pb.GetOrderReply{Id: order.ID}, nil
}
```

## 错误模型

### 业务层只做一件事

```go
return nil, e.NewBusinessErr(defined.OrderNotFound, nil)
```

`CodeErr` 的三个维度分别落到协议的不同位置：

| 维度 | 字段 | 落到线上 |
|---|---|---|
| 类别 | `e.Class`（0~16） | `status.code` |
| 业务码 | `e.Code` | `ErrorInfo.metadata.biz_code` |
| 文案 | `e.Msg` | `status.message` + `ErrorInfo.reason` |

### 线上长什么样

```protobuf
{
  code: 5                          // NOT_FOUND，来自 Class
  message: "Order not found"       // 来自 Msg
  details: [
    {
      "@type": "type.googleapis.com/google.rpc.ErrorInfo",
      reason: "Order not found",   // 业务文案，供排查
      domain: "order-service",     // 服务名，取 ctx，缺失时 "unknown"
      metadata: { "biz_code": "20001" }
    }
  ]
}
```

字段级校验错误会额外挂一条标准 detail：

```protobuf
{ "@type": ".../google.rpc.BadRequest",
  field_violations: [{ field: "amount", description: "amount is required" }] }
```

触发方式：`SetData([]e.FieldViolation{...})`。

### 几个约定

- **`status.code` 只由 `Class` 决定**，不从 `BusinessErr.Err` 透传。需要原样透传下游 status 时，直接 `return statusErr`，拦截器会放行。
- **`reason` 放业务文案**，不放类别常量。类别已经在 `status.code` 里，再放一遍是重复信息，且对端不一致时会出现 `code=NotFound` 但 `reason=INTERNAL` 的自相矛盾。客户端不读 `reason` 判类别。
- **文案只放 `status.message`**，不在 `metadata` 里重复一份。
- **业务码不占 `status.code`**，`status.code` 只有 17 个取值。

### 测试环境暴露底层错误

`Env == test` 时 `status.message` 直接放底层错误，方便联调定位：

```go
msg = bizErr.Err.Error()   // 仅在 Env == test 时
```

生产环境 `message` 恒为 `CodeErr.Msg`，底层错误只进日志和告警。

⚠️ 联调环境必须用非生产数据。

## 内置拦截器

| 拦截器 | 作用 | stream 版 |
|---|---|---|
| `RecoveryUnaryInterceptor()` | recover panic → `SystemErr` | `RecoveryStreamInterceptor()` |
| `RequestIDInterceptor()` | 生成/透传 `x-request-id`，写 ctx + 回写 header | `RequestIDStreamInterceptor()` |
| `LogUnaryInterceptor(logger, opts...)` | method、code、耗时、peer，**不记消息体** | `LogStreamInterceptor()` |
| `AuthInterceptor(verifier)` | 从 metadata 取凭证校验 | `AuthStreamInterceptor()` |
| `RateLimitInterceptor(limiter, keyFunc...)` | 按方法分组限流 | 无 |
| `TimeoutInterceptor(d)` | 无 deadline 时兜一个超时 | 无 |
| `ValidateInterceptor()` | 调 `req.Validate()` | 无 |
| `ErrorUnaryInterceptor()` | 业务错误 → status + details | `ErrorStreamInterceptor()` |

所有拦截器失败时都走 `GRPCErrHandler` 收口，客户端能拿到 `biz_code`。

### 注册顺序

`grpc.ChainUnaryInterceptor` 按注册顺序**由外到内**执行，所以：

- **`ErrorUnaryInterceptor` 必须最后注册**（最内层）。放外面不会改变客户端拿到的结果（有放行判断），但日志会看到未映射的 `code=Unknown`。
- **`RecoveryUnaryInterceptor` 建议最前**，能兜住后面所有拦截器的 panic。
- 拦截器自己返回的错误**不会**再经过内层收口，所以每个拦截器都内部调了 `GRPCErrHandler`。

### 按方法前缀挂载

grpc-go 的拦截器是扁平的，没有 `r.Group()` 那种分组能力，用 `WithMethodPrefix` 补：

```go
base := []grpc.UnaryServerInterceptor{
    grpcserver.RequestIDInterceptor(),
    grpcserver.LogUnaryInterceptor(nil),
}
base = grpcserver.WithMethodPrefix(base, "/order.v1.OrderService/",
    grpcserver.AuthInterceptor(verifier))
```

前缀匹配靠 `info.FullMethod`（`/pkg.Service/Method` 形态）。手写 `grpc.ServiceDesc` 时如果自己伪造 `FullMethod`，前缀会匹配不上，整组拦截器**静默跳过**。

## 选项

### Config

| 字段 | 说明 | 默认 |
|---|---|---|
| `Addr` | 监听地址 | — |
| `MaxRecvMsgSize` | 单条消息最大接收字节 | 4MB |
| `MaxSendMsgSize` | 单条消息最大发送字节 | 4MB |
| `Keepalive` | keepalive 参数 | grpc 默认 |
| `ShutdownTimeout` | 优雅关闭最长等待，超时强制 Stop | 10s |

### Option

| 函数 | 作用 |
|---|---|
| `WithUnaryInterceptor(i)` / `WithUnaryInterceptors(is...)` | 追加 unary 拦截器，按注册顺序执行 |
| `WithStreamInterceptor(i)` / `WithStreamInterceptors(is...)` | 追加 stream 拦截器 |
| `WithCredentials(creds)` | TLS / mTLS |
| `WithMaxMsgSize(recv, send)` | 消息大小上限 |
| `WithKeepalive(params)` | keepalive |
| `WithServerOption(opts...)` | 透传原生 `grpc.ServerOption` |

`WithServerOption` 用于本库没封装的能力，典型的如 OpenTelemetry：

```go
grpcserver.WithServerOption(grpc.StatsHandler(otelgrpc.NewServerHandler()))
```

也可以传 `grpc.ForceServerCodec`、`grpc.MaxConcurrentStreams` 等。

⚠️ 不要再用它传 `grpc.ChainUnaryInterceptor` —— 会和 `WithUnaryInterceptor` 形成两条链，顺序由 grpc 内部决定，不好预测。

### 生命周期

| 方法 | 说明 |
|---|---|
| `GRPC()` | 拿原生 `*grpc.Server`，用于注册 service |
| `Listen()` / `Serve(ctx)` / `ListenAndServe(ctx)` | 启动 |
| `Addr()` | 实际监听地址（`Addr` 传 `:0` 时拿随机端口） |
| `Shutdown(ctx)` | 优雅关闭 |
| `RegisterHealthService()` / `SetHealthStatus(service, serving)` | 健康检查 |
| `RegisterReflection()` | grpcurl 自检 |

## 注意事项

**`GRPCErrHandler` 不幂等。** 对已经是 status 的错误再走一遍，`code` 保留但 `message` 会被替换。`ErrorUnaryInterceptor` 里的 `GRPCStatus()` 判断就是为此加的护栏 —— 它只认直接实现该接口的错误，不能用 `status.FromError`（内部走 `errors.As`，会把「包装了 status 的业务错误」也判成 status，从而跳过映射丢掉业务码）。

**日志不记消息体。** proto 是二进制，可能含敏感字段且体积大。目前也没有开启的开关。metadata 走白名单，`authorization` / `x-token` 等敏感 key 无条件剔除。

**`AuthInterceptor` 从 metadata 取凭证，不是 header。**

**别用 `codes.PermissionDenied` 表示「没登录」。** gRPC 语义区分两者：`PermissionDenied` 是「凭证有效但没权限」，`Unauthenticated` 是「身份无法识别」。内置错误已按此划分：

| 错误 | 含义 | 业务码 |
|---|---|---|
| `MissingCredentialsErr` | 压根没带凭证 | 10101 |
| `AuthErr` / `SignErr` | 带了但无效 | 10105 / 10107 |
| `VerifyErr` / `PermissionErr` | 认证过了但没权限 | 10109 / 10110 |

## 自检

```bash
grpcurl -plaintext localhost:9090 list
grpcurl -plaintext -d '{"id":"1"}' localhost:9090 order.v1.OrderService/GetOrder
```
