# provider/GRPCServer

gRPC 服务端封装。错误处理沿用 go-lib 的**源头归口**模式：业务层只声明错误语义，传输层单一收口点负责日志、告警、协议映射。

## 设计要点

**错误归口不变。** HTTP 侧是 `HTTPErrHandler`，gRPC 侧是 `GRPCErrHandler`，两者职责完全对称：

| | HTTP (`provider/HTTPServer/gin`) | gRPC (`provider/GRPCServer`) |
|---|---|---|
| 业务层怎么写 | `e.NewBusinessErr(e.ParamErr, err)` | **完全一样** |
| 错误提取 | `e.AsBusinessErr(err)` | **同一个** |
| 告警判定 | `Level <= e.WarnLevel` | **同一行逻辑** |
| 告警出口 | `notify.SendErrMessage` | **同一个** |
| 协议表达 | `{code, reason, msg, data}` | `google.rpc.Status` + details |

业务层代码**一行不改**，只是最后的协议表达从 JSON body 变成 gRPC status。

## 错误模型

三个维度各管一件事：

| 维度 | 载体 | 作用 |
|---|---|---|
| **类别** | `e.Class` | 协议层的错误表达 → gRPC `status.code` |
| **业务码** | `e.Code` | 服务内唯一标识一个错误 → `errors.Is` 判定 |
| **文案** | `e.Msg` | 面向用户 → `status.message` + details metadata |

`e.Class` 的取值与 `google.rpc.Code` 完全一致（0~16），所以映射是零成本的查表。

### 线上最终返回什么

```go
// 业务层
return nil, e.NewBusinessErr(defined.OrderNotFound, nil)
```

```protobuf
// 线上传输的 google.rpc.Status
{
  code: 5                          // NOT_FOUND —— 类别
  message: "Order not found"       // 文案
  details: [                       // ← 是数组
    {
      "@type": "type.googleapis.com/google.rpc.ErrorInfo",
      reason: "Data Not Exist",    // 业务文案，方便排查
      domain: "order-service",     // 服务名
      metadata: {
        "biz_code": "20001"         // 业务码（文案只在 status.message 里）
      }
    }
  ]
}
```

### 三个设计决定

**1. 业务码不占 status code。** `status.code` 只有 17 个取值，是「类别」。业务码要能区分同类别的不同错误（「订单不存在」vs「用户不存在」都是 `NOT_FOUND`），必须另辟通道 —— 放在 `ErrorInfo.metadata.biz_code`。

**2. 文案只放 `status.message`，不放 `metadata`。** 同一句话传两遍，一旦不一致（TEST 环境 message 会拼底层错误），客户端该信哪个就成了问题。`CodeErr.Msg` 是编译期常量（`"Order not found"` 这种），真实错误只存在于 `BusinessErr.Err`，它只进日志和告警，不会出现在协议字段里 —— 想泄漏也没有通道。（测试环境例外，见下。）

**2.1 `domain` 取服务名，缺失时填 `"unknown"`。** `domain` 的语义是「错误由哪个服务产生」。没配服务名时不能拿库名（`"go-lib"`）充数 —— 那等于谎称错误来自一个叫 go-lib 的服务，客户端会据此做出错误判断。`"unknown"` 是诚实的表达。

**2.2 `code` 只由 `Class` 决定，不从 `BusinessErr.Err` 透传。** `Err` 的契约是「内部用，不发给客户端」，从它取 code 会让四个字段分成两个来源（实测会产出 `code=NOT_FOUND` + `reason="INTERNAL"` 这种自相矛盾的组合）。而且不能无条件采用底层 status：业务可能在做错误转译（把下游 `Unavailable` 收敛成本服务的 `FailedErr`），覆盖它会改变客户端的重试决策。

需要原样透传下游 status 时，**直接 `return statusErr`**——`ErrorUnaryInterceptor` 里已有放行判断，不包 `BusinessErr` 即可。

**3. `reason` 放业务文案，客户端只认 `status.code`。** 类别由 `status.code`（17 大类）独占表达；`reason` 再放一遍类别是同一信息的两种写法，对端一旦不一致（第三方服务可能这么发），客户端就会出现 `code=NotFound` 但 `reason=INTERNAL` 的自相矛盾。所以 `reason` 改放文案，纯做排查用途，客户端**不拿它判类别**。

这偏离了 AIP-193 对 `reason` 的 UPPER_SNAKE_CASE 约定，是有意的取舍：排查时直接看得懂，不必翻 `biz_code` 对照表。

**4. details 是数组，可挂多种标准 detail。** 除了 `ErrorInfo`，字段级校验错误会挂 `google.rpc.BadRequest`：

```protobuf
details: [
  { "@type": ".../google.rpc.ErrorInfo", reason: "Params Error", ... },
  { "@type": ".../google.rpc.BadRequest",
    field_violations: [{ field: "amount", description: "amount is required" }] }
]
```

全部是 `google.rpc` 标准类型，跨语言客户端开箱即用。

### 测试环境：message 暴露真实错误

`Env == test` 时，`status.message` 直接放底层错误，方便在联调环境定位问题：

```go
// provider/GRPCServer/errors.go
if appctx.OrDefault(ctx).Env == defined.TEST && bizErr.Err != nil {
    msg = bizErr.Err.Error()
}
```

生产环境 `message` 就是 `CodeErr.Msg`（如 `"Operation failed"`），底层错误只进日志和告警。

⚠️ **测试环境必须是非生产数据**。这个分支是刻意保留的排查能力，不是遗漏。

## 「没登录」这类错误怎么表达

`没登录`、`token 过期`、`签名错误` 都是 `UNAUTHENTICATED`（16），靠**业务码**区分：

```go
// err 包内置
MissingCredentialsErr = NewCodeErr(10101, ClassUnauthenticated, "Login credentials missing", InfoLevel)
AuthErr               = NewCodeErr(10105, ClassUnauthenticated, "Auth Error", InfoLevel)
SignErr               = NewCodeErr(10107, ClassUnauthenticated, "Sign Error", InfoLevel)
```

三者语义不同：

| 错误 | 含义 | 客户端动作 |
|---|---|---|
| `MissingCredentialsErr` | 压根没带凭证 | 跳登录页 |
| `AuthErr` / `SignErr` | 带了但无效 | 提示重新认证 |
| `VerifyErr` / `PermissionErr` | 认证过了但没权限 | 提示无权限 |

客户端这样判：

```go
err := client.Invoke(ctx, "/order.v1.OrderService/GetOrder", req, reply)

// Invoke 返回原始 gRPC 错误，业务语义自己取
codeErr, ok := grpcx.ToBusinessError(err)
if !ok {
    // 不是业务错误（纯协议错误或对端没带 biz_code）
    return showError(grpcx.Code(err))
}
if errors.Is(codeErr, e.MissingCredentialsErr) {
    return redirectToLogin()
}
if grpcx.IsUnauthenticated(err) {
    // 所有需要认证的分支都在这
    return showAuthError(codeErr.Code)
}
```

**别用 `codes.PermissionDenied` 表示「没登录」。** gRPC 官方语义明确区分了两者：`PermissionDenied` 是「凭证有效但没权限，重试没用」，`Unauthenticated` 是「身份无法识别，先去拿凭证」。用错会导致客户端做出错误决策（比如该跳登录页却提示无权限）。

## 用法

```go
srv := grpcserver.New(grpcserver.Config{Addr: ":9090"},
    grpcserver.WithUnaryInterceptor(grpcserver.RecoveryUnaryInterceptor()),
    grpcserver.WithUnaryInterceptor(grpcserver.RequestIDInterceptor()),
    grpcserver.WithUnaryInterceptor(grpcserver.LogUnaryInterceptor(nil)),
    grpcserver.WithUnaryInterceptor(grpcserver.AuthInterceptor(verifier)),
    // ★ 错误收口必须放最后（最内层）：业务错误在这里映射成 status
    grpcserver.WithUnaryInterceptor(grpcserver.ErrorUnaryInterceptor()),
    grpcserver.WithStreamInterceptor(grpcserver.RecoveryStreamInterceptor()),
    grpcserver.WithStreamInterceptor(grpcserver.ErrorStreamInterceptor()),   // ★ stream 收口也放最后
)

srv.RegisterHealthService()   // grpc.health.v1 标准协议，K8s 探针直接用
srv.RegisterReflection()       // grpcurl 自检

pb.RegisterOrderServiceServer(srv.GRPC(), impl)
go func() { _ = srv.ListenAndServe(ctx) }()
```

业务实现只需返回 error，映射由 `ErrorUnaryInterceptor` 完成：

```go
func (s *impl) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.GetOrderReply, error) {
    order, err := s.usecase.Get(ctx, req.Id)
    if err != nil {
        return nil, err   // 拦截器映射成正确的 status
    }
    return &pb.GetOrderReply{Id: order.ID}, nil
}
```

## 错误收口：ErrorUnaryInterceptor

业务方法只 `return err`，返回的是 `*e.BusinessErr` 之类的**普通 error**，不是 `*status.Status`。gRPC 框架见到非 status 错误时，会统一包成 `codes.Unknown`，**业务码和 details 全部丢失**，客户端 `errors.Is` 直接失效。

`ErrorUnaryInterceptor` 就是补这一步的：

```go
func ErrorUnaryInterceptor() grpc.UnaryServerInterceptor
```

它在业务方法返回后做最后一次映射：

1. 已经是 `*status.Status` 的错误**原样放行**（鉴权/限流/超时拦截器自己造的 status 不能被二次加工）
2. 其余错误交给 `GRPCErrHandler` 收口：归一化 → 按级别告警 → 映射成 status + details

### 必须放在链尾

```go
srv := grpcserver.New(grpcserver.Config{Addr: ":9090"},
    grpcserver.WithUnaryInterceptor(grpcserver.RecoveryUnaryInterceptor()),  // 最外层
    grpcserver.WithUnaryInterceptor(grpcserver.RequestIDInterceptor()),
    grpcserver.WithUnaryInterceptor(grpcserver.LogUnaryInterceptor(nil)),
    grpcserver.WithUnaryInterceptor(grpcserver.AuthInterceptor(verifier)),
    grpcserver.WithUnaryInterceptor(grpcserver.ErrorUnaryInterceptor()),     // ★ 最内层
)
```

`grpc.ChainUnaryInterceptor` 按注册顺序**由外到内**执行，所以收口要**最后注册**。放错了不会报错，但有两个静默后果：

| 顺序 | 后果 |
|---|---|
| 收口在内层（正确） | 日志看到映射后的 `code=NotFound` |
| 收口在外层（错误） | 日志看到 `code=Unknown`；且鉴权失败的文案会被 `FailedErr.Msg` 覆盖成 `"Operation failed"` |

第二条的根因是 **`GRPCErrHandler` 不幂等**：对已经是 status 的错误再走一遍，code 保留但 message 会被替换。拦截器里的 `GRPCStatus()` 判断就是为此加的护栏。

### 为什么不用 status.FromError 判断

```go
// ✅ 只认直接实现 GRPCStatus 的错误（status.Error 的返回值）
if _, ok := err.(interface{ GRPCStatus() *status.Status }); ok {
    return resp, err
}

// ❌ status.FromError 内部走 errors.As，
//    会把「包装了 status 的业务错误」也判成 status，
//    从而跳过映射、丢掉业务码。
```

比如 `fmt.Errorf("查订单失败: %w", bizErr)`，`errors.As` 能穿透到里面的 status，`status.FromError` 就返回 `ok=true` 了。但它是业务错误，必须走收口。

### 它替代了什么

早期版本里有一个 `UnaryHandler` 泛型函数，专门给「手写 `grpc.ServiceDesc`」的场景做收口。它已被删除，原因：

- **零调用点**：仓库里没有任何地方用它
- **protoc 路径根本不经过它**：README 的主用法是 `pb.RegisterOrderServiceServer(srv.GRPC(), impl)`，生成代码直接调业务方法，`UnaryHandler` 完全没机会执行 —— 也就是说收口在真实链路上从来没生效过
- **`FullMethod` 是类型名**：`grpc.MethodDesc.Handler` 的签名拿不到方法名，`UnaryHandler` 只能用反射从接收者类型推断，得到 `github.com/.../grpcserver.OrderService` 而不是 `/order.v1.OrderService/GetOrder`，导致 `WithMethodPrefix` 静默失效

拦截器方案一次性解决这三个问题：它对两条注册路径都生效，且 `info.FullMethod` 由框架传入，是真实方法名。

### 和 handler.CD 的分工

| | HTTP `handler.CD` | gRPC `ErrorUnaryInterceptor` |
|---|---|---|
| 参数绑定 | **自己做**（松散的 JSON） | 交给 `dec`（`.proto` 是编译期契约） |
| 参数校验 | `ValidateHook` 反射扫字段 | `ValidateInterceptor` 调 `req.Validate()` |
| 错误收口 | 包在 CD 里 | 包在拦截器里 |
| 响应包装 | 统一 JSON body | 不需要，返回 proto message 即可 |

gRPC 侧薄，是因为绑定和契约由生成代码承担了；HTTP 侧厚，是因为散装 JSON 没有契约。

### 边界行为

| 情况 | 结果 |
|---|---|
| 业务返回 `nil` | 原样返回响应，不包装 |
| 业务返回业务错误 | 映射成对应 class 的 status，`biz_code` 进 details |
| 业务返回包装过的业务错误 | 同上（`errors.As` 能穿透） |
| 业务返回裸 error | `Internal` / `"Operation failed"`（`FailedErr.Msg`），原始错误只进日志 |
| 业务返回包装过的 status | 同样归到 `Internal`：`code` 只看 `Class`，不从 `Err` 透传 |
| 业务返回 `*status.Status` | **原样放行**，`code`/`message`/`details` 全保留（想透传下游 status 就用这个） |
| 业务 panic | 由 `RecoveryUnaryInterceptor` 兜，与收口无关 |

## 内置拦截器

| 拦截器 | 作用 |
|---|---|
| `RecoveryUnaryInterceptor()` | recover panic → `codes.Internal`，堆栈只进日志 |
| `RecoveryStreamInterceptor()` | 同上，stream 版 |
| `RequestIDInterceptor()` | 生成/透传 `x-request-id`，写 ctx + 回写 header |
| `LogUnaryInterceptor(logger, opts...)` | method、code、耗时、peer；**不记消息体** |
| `AuthInterceptor(verifier)` | metadata 取凭证 → 校验 → `codes.Unauthenticated` |
| `RateLimitInterceptor(limiter, keyFunc...)` | 按方法分组限流 → `codes.ResourceExhausted` |
| `TimeoutInterceptor(d)` | 无 deadline 时兜一个超时 |
| `ValidateInterceptor()` | 调 `req.Validate()` → `codes.InvalidArgument` |
| `ErrorUnaryInterceptor()` | 业务错误 → status + details（**必须最后注册**） |
| `ErrorStreamInterceptor()` | 同上，stream 版 |

### 按方法前缀挂载

grpc-go 的拦截器是扁平的，没有 HTTP 里 `r.Group("/v1", Auth(token))` 那种分组能力。用 `WithMethodPrefix` 补齐：

```go
base := grpcserver.WithUnaryInterceptor(grpcserver.RequestIDInterceptor())
base = grpcserver.WithMethodPrefix(base, "/order.v1.OrderService/",
    grpcserver.AuthInterceptor(verifier))
```

前缀匹配靠 `info.FullMethod`，也就是 `/pkg.Service/Method` 形态的完整方法名。
手写 `grpc.ServiceDesc` 时如果自己伪造 `FullMethod`（比如填成类型名），
前缀会匹配不上，整组拦截器被**静默跳过** —— 没有任何报错，表现为「鉴权莫名不生效」。
走 `protoc` 生成的注册路径则由框架填真实方法名，不存在这个问题。

## 不记消息体

HTTP 侧能记 request_body / response_body，gRPC 侧**不记**：proto 是二进制，可能含敏感字段且体积大。

**目前没有开启消息体记录的开关。** 真要做需要单独设计（字段白名单 + 截断 + 脱敏），不能顺手加一个全局开关。

metadata 走白名单（默认只记 `content-type`、`user-agent`），`authorization` / `x-token` 等敏感 key 无条件剔除。

## 自检

```bash
grpcurl -plaintext localhost:9090 list
grpcurl -plaintext -d '{"id":"1"}' localhost:9090 order.v1.OrderService/GetOrder
```
