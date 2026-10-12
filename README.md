# go-lib — Go 后端通用工具库

个人积累的 Go 后端开发工具库，封装了日常开发中高频使用的模块：GORM Scope 声明式查询、错误堆栈追踪、Gin 泛型控制器、HTTP 洋葱模型中间件链等。

## 安装

```bash
go get github.com/Cotary/go-lib@latest
```

---

## 核心模块速览

| 模块 | 路径 | 功能 |
|------|------|------|
| **GORM Scope** | `dao/gormDB` | 声明式查询条件 + `filter` tag 自动构建 + 分页/排序 |
| **错误追踪** | `err` | `e.Err()` 一行包裹错误并附带完整调用堆栈 |
| **Gin 控制器** | `provider/HTTPServer/gin` | `CD` 泛型控制器，自动参数绑定 + 统一响应格式 |
| **HTTP 客户端** | `net/http` | 洋葱模型中间件链式调用，内置重试/认证/限流/日志 |
| **分布式锁** | `dlock` | Redis 分布式锁，自动续期 |
| **缓存** | `cache` | Redis 泛型缓存封装 |
| **消息通知** | `provider/message` | Telegram / 飞书 / 邮件统一发送 |
| **Excel 导出** | `provider/exporter` | 结构化数据导出 Excel |
| **节点池** | `provider/nodepool` | 多节点负载均衡 + 健康检查 + HTTP 传输层 |
| **日志** | `log` | Zap 日志封装，结构化日志 + Context 追踪 |
| **配置** | `config` | Viper YAML 配置加载 + 结构体绑定 |

---

## 1. GORM Scope — 声明式查询构建

将查询条件封装为可组合的 `Scope` 函数，通过 `db.Scopes()` 链式叠加，告别 `if-else` 堆砌 WHERE 条件。

### 手动 Scope 组合

```go
import "github.com/Cotary/go-lib/dao/gormDB"

// 每个 Scope 是一个 func(*gorm.DB) *gorm.DB，可自由组合
users, err := gormDB.PageList[User](ctx, db, &req.Paging,
    gormDB.WhereIf("status", gormDB.OpEq, req.Status),       // 零值跳过，非零生效
    gormDB.WhereAlways("type", gormDB.OpEq, req.Type),        // 零值也生效（type=0 有业务含义）
    gormDB.WhereNullable("parent_id", gormDB.OpEq, req.ParentID), // nil → IS NULL
    gormDB.WhereIf("name", gormDB.OpLike, req.Keyword),       // 模糊搜索，自动转义通配符
    gormDB.WhereIf("role", gormDB.OpIn, req.Roles),           // IN 查询，空切片自动跳过
    gormDB.OrderWhitelist(req.Order, map[string]string{        // 白名单排序，防 SQL 注入
        "created_at": "t.created_at",
        "score":      "t.score",
    }),
)
```

### `filter` tag 自动构建（推荐）

在请求 DTO 上打 tag，一行代码自动生成所有 Where + 分页 + 排序：

```go
type ListReq struct {
    Name     string   `filter:"name,like"`               // 模糊搜索，零值跳过
    Status   *int64   `filter:"status,=,always"`          // nil 折回零值，始终生效
    ParentID *int64   `filter:"parent_id,=,nullable"`     // nil → IS NULL
    Keyword  string   `filter:"name|description,like"`    // 多列 OR 模糊搜索
    Roles    []string `filter:"role,in"`                  // IN 查询
    community.Paging                                       // 自动分页 + count
    community.Order                                        // 自动排序
}

// 一行搞定
users, err := gormDB.PageList[User](ctx, db, &req.Paging,
    gormDB.ApplyFilter(req, map[string]string{"created_at": "t.created_at"}),
)
```

### 泛型 CRUD

```go
// 查询单条
user, err := gormDB.Get[User](ctx, db, gormDB.ID(123))

// 查询单条（不存在返回零值而非 error）
user, err := gormDB.OptionalGet[User](ctx, db, gormDB.WhereIf("email", gormDB.OpEq, email))

// 查询列表
users, err := gormDB.List[User](ctx, db, gormDB.WhereIf("age", gormDB.OpGte, 18))

// 分页查询（自动 count + 填充 Total）
users, err := gormDB.PageList[User](ctx, db, &paging, scopes...)

// Upsert（ON CONFLICT）
err := db.Save(ctx, &user, []string{"name", "email"}, "unique_key")

// 先查后写（事务内加锁查询 + 插入或更新）
op, err := db.QueryAndSave(ctx, &user, []string{"*"}, map[string]any{"email": user.Email})
// op == OperationInsert / OperationUpdate / OperationNothing
```

---

## 2. e.Err — 错误堆栈追踪

一行代码给 error 附加完整调用堆栈，链路追踪时直接定位出错位置：

```go
import e "github.com/Cotary/go-lib/err"

func CreateUser(req *CreateReq) error {
    user, err := db.Insert(ctx, req)
    if err != nil {
        return e.Err(err)  // 自动附加堆栈，已有堆栈的不会重复包裹
    }
    return nil
}

func Handler() error {
    err := CreateUser(req)
    if err != nil {
        return e.Err(err, "创建用户失败")  // 可叠加消息，堆栈不会重复
    }
    return nil
}

// 打印完整错误链 + 堆栈
fmt.Println(e.GetErrMessage(err, false))
// 输出:
// [1]: 创建用户失败: duplicate key
//
// /app/service/user.go:42
// /app/handler/user.go:15
// ...
```

### HTTP 错误码

```go
// 定义业务错误码
var ParamErr = e.NewCodeErr(400, "参数错误", e.WarnLevel)
var AuthErr  = e.NewCodeErr(401, "未授权",   e.WarnLevel)

// 在 handler 中使用
return e.NewHttpErr(e.ParamErr, err).SetData(err.Error())
// 响应: {"code": 400, "message": "参数错误", "data": "name is required"}
```

---

## 3. Gin CD — 泛型控制器

`CD` 将 Gin handler 封装为泛型函数：自动参数绑定、自动错误处理、统一 JSON 响应格式。

```go
import "github.com/Cotary/go-lib/provider/HTTPServer/gin/handler"

type CreateUserReq struct {
    Name  string `json:"name" binding:"required"`
    Email string `json:"email" binding:"required,email"`
    Age   int    `json:"age"`
}

type UserResp struct {
    ID    int64  `json:"id"`
    Name  string `json:"name"`
}

// CD 自动完成: ShouldBind → Validate → 执行 → 统一响应
r.POST("/user/create", handler.CD(func(c *gin.Context, req CreateUserReq) (*UserResp, error) {
    user, err := service.CreateUser(ctx, &req)
    if err != nil {
        return nil, e.Err(err)
    }
    return &UserResp{ID: user.ID, Name: user.Name}, nil
}))

// 成功响应: {"code": 0, "message": "success", "data": {"id": 1, "name": "test"}}
// 失败响应: {"code": 500, "message": "创建失败"}
// 参数错误: {"code": 400, "message": "参数错误", "data": "Name is required"}
```

### 内置 Hook 链

`CD` 内置 `AutoBind → Validate → 用户自定义 Hook → Handler` 的处理链，可扩展自定义 Hook（如缓存、权限校验等）。

### 配套中间件

```go
// 标准 Gin 服务启动，内置 Recovery + CORS + 请求日志 + 优雅关闭
gin.Use(
    handler.RequestID(),                    // 请求 ID 注入
    gin.CustomRecoveryWithWriter(nil, handler.RecoveryHandler()),  // Panic 恢复 + 堆栈上报
    handler.CorsHandler(),                  // CORS 跨域
    handler.RequestLogHandler(),            // 请求/响应日志
)
```

---

## 4. HTTP 客户端 — 洋葱模型中间件链

类似 Gin 的中间件设计，HTTP 客户端也采用洋葱模型：请求穿过中间件层发出，响应反向穿过中间件层返回。

```go
import http2 "github.com/Cotary/go-lib/net/http"

// 创建带中间件的 HTTP 客户端
client := http2.FastHTTP().
    Use(
        http2.RecoveryMiddleware(),                       // 最外层：panic 恢复
        http2.RetryMiddleware(3, time.Second),            // 自动重试 3 次
        http2.AuthMiddleware("appId", "secret", "md5"),   // 签名认证
        http2.TimeoutMiddleware(10 * time.Second),        // 超时控制
        http2.StatusCodeCheckMiddleware(),                // 状态码校验（非 2xx 报错）
    )

// 执行请求 + 解析响应（链式调用）
var user User
err := client.Execute(ctx, "POST", "https://api.example.com/user",
    nil,                                    // query params
    map[string]any{"name": "test"},         // body (自动 JSON 序列化)
    map[string]string{"X-Token": token},    // headers
).ParseTo("data.user", &user)

// 泛型解析
user, err := http2.Parse[User](client.Execute(ctx, "GET", url, nil, nil, nil), "data")
```

### 内置中间件

| 中间件 | 功能 |
|--------|------|
| `RecoveryMiddleware` | panic 恢复 |
| `RetryMiddleware` | 自动重试（5xx 重试，4xx 不重试） |
| `AuthMiddleware` | 应用签名认证 |
| `TimeoutMiddleware` | 请求超时 |
| `RateLimitMiddleware` | 令牌桶限流 |
| `StatusCodeCheckMiddleware` | HTTP 状态码校验 |
| `CodeCheckMiddleware` | 业务状态码校验（解析 JSON 中的 code 字段） |
| `LoggingMiddleware` | 请求/响应结构化日志 |
| `TracingMiddleware` | 请求 ID 追踪 |
| `HeaderMiddleware` | 自定义请求头注入 |

支持 FastHTTP（高性能）和 Resty（功能丰富）两种底层引擎。

---

## 5. 其他模块

### 节点池 — 多节点负载均衡

```go
import "github.com/Cotary/go-lib/provider/nodepool"

pool := nodepool.New(
    nodepool.WithNodes("https://rpc1.example.com", "https://rpc2.example.com"),
    nodepool.WithStrategy(nodepool.RoundRobin),  // 轮询 / 随机 / 权重
    nodepool.WithHealthCheck(30 * time.Second),  // 自动健康检查
)
node := pool.Get()  // 获取一个健康节点
```

### 分布式锁

```go
import "github.com/Cotary/go-lib/dlock"

lock := dlock.New(redisClient, "order:123", 30*time.Second)
if err := lock.Lock(ctx); err != nil {
    return err
}
defer lock.Unlock(ctx)
```

### 消息通知

```go
import "github.com/Cotary/go-lib/provider/message"

// 统一接口，支持 Telegram / 飞书
message.Send(ctx, "服务异常告警: xxx")
```

---

## 项目结构

```
go-lib/
├── err/                    # 错误追踪（e.Err + 堆栈 + HTTP 错误码）
├── dao/
│   ├── gormDB/             # GORM Scope 声明式查询 + filter tag + 泛型 CRUD
│   ├── redis/              # Redis 客户端封装
│   ├── mongo/              # MongoDB 客户端封装
│   └── rabbitMQ/           # RabbitMQ 连接与队列管理
├── net/
│   ├── http/               # HTTP 客户端（洋葱模型中间件 + FastHTTP/Resty 双引擎）
│   └── ws/                 # WebSocket 客户端/服务端
├── provider/
│   ├── HTTPServer/gin/     # Gin 泛型控制器（CD）+ 中间件 + 导出
│   ├── nodepool/           # 多节点负载均衡 + 健康检查
│   ├── message/            # 消息通知（Telegram / 飞书）
│   ├── exporter/           # Excel 导出
│   ├── email/              # 邮件发送
│   └── chain/              # 区块链工具（ETH / TRON / Solana / BTC）
├── cache/                  # Redis 泛型缓存
├── dlock/                  # Redis 分布式锁
├── log/                    # Zap 日志封装
├── config/                 # Viper 配置加载
├── notify/                 # 错误通知
├── common/                 # 通用工具（类型转换、分页、加密、时间等）
├── cmd/                    # HTTP 服务启动
└── export/                 # 导出工具
```

## 语言

Go 100%
