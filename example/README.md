# go-lib 使用指南

go-lib 是通用 Go 基础库，为业务项目提供开箱即用的基础设施：日志、配置、数据库、缓存、HTTP 客户端、Web 框架增强、错误处理、定时任务、消息队列、分布式锁等。

本目录通过一个虚构的 `myproject`（订单系统）展示 go-lib 的标准接入方式。所有代码仅供阅读参考，不可直接编译运行。

---

## 目录

1. [快速开始](#1-快速开始)
2. [推荐项目结构](#2-推荐项目结构)
3. [配置管理](#3-配置管理)
4. [Model 层](#4-model-层)
5. [错误处理](#5-错误处理)
6. [服务启动](#6-服务启动)
7. [路由与中间件](#7-路由与中间件)
8. [请求与响应 DTO](#8-请求与响应-dto)
9. [业务逻辑（Logic）](#9-业务逻辑logic)
10. [Provider 外部调用](#10-provider-外部调用)
11. [数据源初始化](#11-数据源初始化)
12. [定时任务](#12-定时任务)
13. [通用工具](#13-通用工具)

---

## 1. 快速开始

### 引入 go-lib

远程引用：

```
require github.com/Cotary/go-lib latest
```

本地开发（go.mod replace）：

```
replace go-lib => ./go-lib

require go-lib v0.0.0
```

使用 replace 时，import 路径统一写 `go-lib/...`，如 `go-lib/dao/gormDB`、`go-lib/err`。

### 初始化模板

每个服务的 `init()` 遵循相同流程，参见 [server/api/main.go](server/api/main.go)：

```go
lib.Init(config.Config.ServerName, config.Config.ENV)       // 1. 服务名 + 环境
lib.InitLog(log2.NewLogger(config.Config.Logging))           // 2. 全局日志
flush := lib.BootstrapCrashCapture()                         // 3. 崩溃捕获
// lib.InitGlobalSender(sender)                              // 4. 告警 Sender（按需）
flush(coroutines.NewContext("crash-report"))                  // 5. 补报历史 crash
```

---

## 2. 推荐项目结构

```
myproject/
├── config/                    # 共享配置结构定义 + 加载
│   └── config.go
├── model/                     # 数据模型（按库分子目录，如 model/core/、model/backend/）
│   ├── resources.go           #   DB 初始化 + 表名常量
│   └── order.go               #   Model 定义
├── common/                    # 跨服务通用代码
│   └── defined/
│       ├── consts.go          #   常量（Redis key、枚举）
│       └── errors.go          #   业务错误码
├── dao/                       # 数据源初始化（Redis、MQ 等）
│   └── dao.go
├── provider/                  # 外部服务 / 领域能力封装
│   └── external_api.go
└── server/                    # 可部署服务（每个子目录 = 独立服务）
    ├── api/                   #   HTTP 服务
    │   ├── main.go            #     启动入口
    │   ├── config/            #     该服务专属配置模板
    │   ├── router/            #     路由注册
    │   ├── middleware/        #     中间件
    │   ├── community/         #     请求/响应 DTO
    │   ├── logic/             #     业务逻辑
    │   └── cmd/               #     定时任务
    ├── admin/                 #   管理后台服务（结构同 api）
    └── cron/                  #   纯定时任务服务
```

**核心原则：**

- 外层目录（config、model、common、dao、provider）是所有服务共享的基础设施
- `server/` 下每个子目录是独立可部署服务，有自己的 main.go 和子目录
- 不绑定特定架构——简单项目可以只有一个 server，复杂项目可按需拆分 HTTP / RPC / Cron 等多个服务
- 每个服务有自己的 `config/` 目录存放配置模板，外层 `config/config.go` 定义共享配置结构

---

## 3. 配置管理

> 参见 [config/config.go](config/config.go) + [server/api/config/config.yaml](server/api/config/config.yaml)

### 共享配置结构

外层 `config/config.go` 定义一个聚合的 `Conf` 结构体，引用 go-lib 提供的配置类型：

```go
type Conf struct {
    ServerPort string             `yaml:"serverPort"`
    ServerName string             `yaml:"serverName"`
    ENV        string             `yaml:"env"`
    Logging    *log2.Config       `yaml:"log"`
    DB         *gormDB.GormConfig `yaml:"db"`
    Redis      *redis.Config      `yaml:"redis"`
    RabbitMQ   *rabbitMQ.Config   `yaml:"rabbitMQ"`
    // ... 业务自定义配置
}
```

使用 `config.Parse` 加载：

```go
config.Parse("./config.yaml", "", Config)
```

### 各服务独立配置

每个服务在 `config/` 或 `config_template/` 目录下有自己的 yaml，只填该服务需要的字段。例如纯 HTTP 服务不需要 MQ 配置，定时任务服务不需要 ServerPort。

---

## 4. Model 层

> 参见 [model/resources.go](model/resources.go) + [model/order.go](model/order.go)

这是最核心的部分，所有数据库操作都围绕 Model + gormDB Scope 模式展开。

### Model 定义四件套

每个 Model 文件遵循统一结构：

**1) 嵌入基类 + 字段定义（必须有行尾中文注释）：**

```go
type Order struct {
    gormDB.BaseModel                              // 包含 ID、CreatedAt、UpdatedAt
    UserID  int64  `gorm:"column:user_id"`        // 所属用户ID
    Status  uint32 `gorm:"column:status"`         // 订单状态：0-待支付 1-已支付 2-失败 3-已退款
    Amount  string `gorm:"column:amount"`         // 订单金额
}
```

**2) 构造函数：**

```go
func NewOrder() *Order { return &Order{} }
```

**3) TableName：**

```go
func (Order) TableName() string { return TableNameOrder }
```

**4) CRUD 方法，委托 gormDB 泛型函数：**

```go
func (m *Order) Get(ctx context.Context, scopes ...gormDB.Scope) (Order, error) {
    return gormDB.Get[Order](ctx, DBDriver, scopes...)
}
func (m *Order) List(ctx context.Context, scopes ...gormDB.Scope) ([]Order, error) {
    return gormDB.List[Order](ctx, DBDriver, scopes...)
}
func (m *Order) PageList(ctx context.Context, p *community.Paging, scopes ...gormDB.Scope) ([]Order, error) {
    return gormDB.PageList[Order](ctx, DBDriver, p, scopes...)
}
```

### gormDB Scope 查询模式

go-lib 提供的核心 Scope 函数：

| 函数 | 用途 | 零值行为 |
|------|------|----------|
| `WhereIf(col, op, val)` | 通用条件 | 零值/nil 跳过 |
| `WhereAlways(col, op, val)` | 零值有意义的条件 | 零值也生效（如 status=0） |
| `WhereNullable(col, op, val)` | NULL 语义 | nil → IS NULL |
| `Paginate(p)` | 分页 + count | p 为 nil 时 no-op |
| `OrderWhitelist(o, mapping)` | 安全排序 | 白名单外的字段被忽略 |
| `ID(id)` | 主键查询 | - |

**调用方式——在 logic 层组合 Scope：**

```go
list, err := model.NewOrder().PageList(ctx, &req.Paging,
    gormDB.WhereIf("user_id", gormDB.OpEq, req.UserID),     // 有值才过滤
    gormDB.WhereIf("status", gormDB.OpEq, req.Status),      // 指针 nil 时跳过
    gormDB.WhereIf("trade_no", gormDB.OpLike, req.Keyword), // 模糊搜索
    gormDB.OrderWhitelist(req.Order, map[string]string{      // 白名单排序
        "created_at": "created_at",
        "amount":     "amount",
    }),
)
```

### 操作符列表

| 操作符 | SQL |
|--------|-----|
| `OpEq` | `=` |
| `OpNeq` | `<>` |
| `OpLt / OpLte` | `< / <=` |
| `OpGt / OpGte` | `> / >=` |
| `OpIn / OpNotIn` | `IN / NOT IN` |
| `OpLike / OpNotLike` | `LIKE / NOT LIKE` |

### DB 初始化

集中在 `model/resources.go`：

```go
var DBDriver *gormDB.GormDrive

func Init() {
    DBDriver = gormDB.MustNewGorm(config.Config.DB)
}
```

---

## 5. 错误处理

> 参见 [common/defined/errors.go](common/defined/errors.go)

### 双层错误体系

**go-lib 内置错误**（`go-lib/err` 包，别名 `e`）：

- `e.ParamErr` — 参数错误 (10003)
- `e.DataNotExist` — 数据不存在 (10004)
- `e.AuthErr` — 认证失败 (10006)
- `e.SystemErr` — 系统异常 (10001)
- 完整列表见 `go-lib/err/common.go`

**业务项目自定义错误码**（从 20001 起步）：

```go
const (
    OrderNotFoundCode = iota + 20001
    OrderAlreadyPaidCode
    InsufficientBalanceCode
)

var (
    OrderNotFound    = e.NewCodeErr(OrderNotFoundCode, "Order not found", e.InfoLevel)
    OrderAlreadyPaid = e.NewCodeErr(OrderAlreadyPaidCode, "Order already paid", e.InfoLevel)
)
```

### 使用规范

```go
// 返回业务错误给客户端
return nil, e.NewHttpErr(defined.OrderNotFound, nil)

// 携带底层错误（记日志，不暴露给客户端）
return nil, e.NewHttpErr(defined.payGatewayErr, err)

// 包装底层错误向上传播（保留堆栈）
return nil, e.Err(err)

// 使用 go-lib 内置错误
return nil, e.NewHttpErr(e.ParamErr, errors.New("amount is required"))
```

**重要规范：**

- error 文案**必须英文**（便于 ELK/Sentry 聚合和 grep）
- **禁止 `_` 丢弃 error**，必须处理或向上传播
- 非致命错误可用 `notify.SendErrMessage` 报警后 `continue`

---

## 6. 服务启动

> 参见 [server/api/main.go](server/api/main.go)

### Gin 中间件链

标准中间件链按以下顺序挂载：

```go
r := gin.New()
r.Use(gin.CustomRecovery(handler.RecoveryHandler()))  // panic 恢复 + 告警
r.Use(handler.CorsMiddleware())                        // CORS 跨域
r.Use(handler.RequestIDMiddleware())                   // 注入 RequestID 到 context
r.Use(handler.RequestLogMiddleware())                  // 请求/响应日志
```

### 优雅退出

监听 SIGINT/SIGTERM 信号，调用 `srv.Shutdown` 等待请求处理完毕后退出。

---

## 7. 路由与中间件

> 参见 [server/api/router/router.go](server/api/router/router.go) + [server/api/middleware/auth.go](server/api/middleware/auth.go)

### 路由命名规范

```
格式：/api/{模块}/{动作}
示例：
  GET  /api/order/detail?id=123          → 订单详情
  GET  /api/order/list?page=1&status=1   → 订单列表
  POST /api/order/create                 → 创建订单
```

- 统一前缀 `/api`
- 模块名和动作名使用 **kebab-case**（如 `/api/energy-rental/config`）
- GET 请求通过 `?` 传参，POST 请求通过 JSON body 传参

### handler.CD 控制器包装

`handler.CD` 是核心的控制器包装函数，自动完成参数绑定和响应格式化：

```go
order.GET("/detail", handler.CD(logic.OrderDetail))
order.POST("/create", handler.CD(logic.CreateOrder))
```

它的作用：
1. 自动将请求参数绑定到 `req T`（GET 用 `form` 标签，POST 用 `json` 标签）
2. 绑定失败自动返回 `ParamErr`
3. 成功时包装为 `{"code":0, "msg":"success", "data": ...}` 响应
4. 出错时通过 `AbortWithError` 统一处理

### 中间件挂载

按路由组挂载中间件，不同模块可使用不同的鉴权策略：

```go
order := api.Group("/order", middleware.Auth())       // 需要登录
public := api.Group("/public")                        // 公开接口
```

---

## 8. 请求与响应 DTO

> 参见 [server/api/community/order.go](server/api/community/order.go)

### GET 请求 DTO — `form` 标签

GET 请求参数通过 URL 查询字符串传递，DTO 字段**必须使用 `form` 标签**：

```go
// GET /api/order/detail?id=123
type OrderDetailRequest struct {
    ID int64 `form:"id" binding:"required"`
}
```

### POST 请求 DTO — `json` 标签

POST 请求参数通过 JSON body 传递，DTO 字段使用 `json` 标签，`binding` 标签做校验：

```go
// POST /api/order/create  body: {"amount":"100","currency":"USD"}
type CreateOrderRequest struct {
    Amount   string `json:"amount" binding:"required"`
    Currency string `json:"currency" binding:"required"`
}
```

### 分页请求

嵌入 `community.Paging` 和 `community.Order`（它们已有 `form` + `json` 标签）：

```go
type OrderListRequest struct {
    community.Paging                       // page, page_size, all
    community.Order                        // order_field, order_type
    Status  *uint32 `form:"status"`        // 指针类型，nil 时 WhereIf 自动跳过
    Keyword string  `form:"keyword"`
}
```

### 分页响应

使用 `community.PageOf` 一行构造：

```go
list, err := model.NewOrder().PageList(ctx, &req.Paging, scopes...)
return community.PageOf(list, req.Paging), nil
```

响应格式：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "list": [...],
    "page": 1,
    "page_size": 10,
    "total": 100
  }
}
```

---

## 9. 业务逻辑（Logic）

> 参见 [server/api/logic/order.go](server/api/logic/order.go)

### logic 函数签名

配合 `handler.CD` 使用，函数签名固定为：

```go
func XxxAction(c *gin.Context, req RequestType) (ResponseType, error)
```

### 三种典型 logic

**列表查询（分页 + Scope 组合）：**

```go
func OrderList(c *gin.Context, req apiDto.OrderListRequest) (*community.ListPageResponse, error) {
    ctx := c.Request.Context()
    list, err := model.NewOrder().PageList(ctx, &req.Paging,
        gormDB.WhereIf("user_id", gormDB.OpEq, req.UserID),
        gormDB.WhereIf("status", gormDB.OpEq, req.Status),
    )
    if err != nil { return nil, e.Err(err) }
    return community.PageOf(list, req.Paging), nil
}
```

**详情查询（GET 参数 + 错误处理）：**

```go
func OrderDetail(c *gin.Context, req apiDto.OrderDetailRequest) (*model.Order, error) {
    order, err := model.NewOrder().Get(ctx, gormDB.ID(req.ID))
    if err != nil {
        if gormDB.DbErr(err) == nil {
            return nil, e.NewHttpErr(defined.OrderNotFound, nil)  // 业务错误
        }
        return nil, e.Err(err)  // 数据库错误
    }
    return &order, nil
}
```

**创建操作（POST body + 外部调用）：**

```go
func CreateOrder(c *gin.Context, req apiDto.CreateOrderRequest) (*apiDto.CreateOrderResponse, error) {
    order := &model.Order{Amount: req.Amount, ...}
    if err := order.Insert(ctx); err != nil { return nil, e.Err(err) }
    payURL, err := provider.Createpay(ctx, ...)
    if err != nil { return nil, e.NewHttpErr(defined.payGatewayErr, err) }
    return &apiDto.CreateOrderResponse{payURL: payURL}, nil
}
```

---

## 10. Provider 外部调用

> 参见 [provider/external_api.go](provider/external_api.go)

使用 `go-lib/net/http` 的链式 API 调用外部 HTTP 服务：

```go
var status string
err := http2.FastHTTP().
    Use(
        http2.TimeoutMiddleware(10 * time.Second),   // 超时
        http2.CodeCheckMiddleware(0),                 // 校验业务 code=0
    ).
    Execute(ctx, http.MethodGet, url, query, nil, nil).
    ParseTo("data.status", &status)
```

**两种 HTTP 客户端：**

| 工厂函数 | 底层引擎 | 适用场景 |
|----------|----------|----------|
| `http2.FastHTTP()` | valyala/fasthttp | 高并发、性能优先 |
| `http2.RestyHTTP()` | go-resty | 功能丰富、调试方便 |

**常用中间件：**

| 中间件 | 用途 |
|--------|------|
| `TimeoutMiddleware(d)` | 请求超时 |
| `CodeCheckMiddleware(code)` | 校验响应 JSON 中的业务 code |
| `StatusCodeCheckMiddleware(codes...)` | 校验 HTTP 状态码 |
| `RetryMiddleware(max, delay)` | 自动重试（5xx） |
| `AuthMiddleware(appID, secret, signType)` | 签名认证 |
| `RecoveryMiddleware()` | panic 恢复 |

---

## 11. 数据源初始化

> 参见 [dao/dao.go](dao/dao.go)

数据源（Redis、MQ 等）在 `dao/` 包中集中初始化：

```go
var Redis redis.Client

func InitRedis() {
    var err error
    Redis, err = redis.NewRedis(config.Config.Redis)
    if err != nil { panic(err) }
}
```

各服务在 `init()` 或 `main()` 中按需调用：

```go
dao.InitRedis()
model.Init()    // 数据库
dao.InitMQ()    // MQ（如果需要）
```

Redis 操作时使用 `Key()` 方法自动拼接前缀：

```go
dao.Redis.Get(ctx, dao.Redis.Key("order:lock:" + orderID))
```

---

## 12. 定时任务

> 参见 [server/api/cmd/order_timeout.go](server/api/cmd/order_timeout.go)

### 实现 cmd.Handler 接口

```go
type OrderTimeoutJob struct{}

func (j *OrderTimeoutJob) Spec() string              { return "0 */5 * * * *" }     // 6 位 cron
func (j *OrderTimeoutJob) MaxExecuteTime() time.Duration { return 2 * time.Minute } // 超时告警
func (j *OrderTimeoutJob) Do(ctx context.Context) error  { /* 业务逻辑 */ }
```

### 注册并启动

```go
scheduler, _ := cmd.NewScheduler()
scheduler.AddJob("order-timeout-check", &OrderTimeoutJob{})
scheduler.Start()
```

**内置特性：**

- SingletonMode：同一任务不会并发执行
- 超时告警：超过 `MaxExecuteTime` 自动告警
- Panic Recovery：任务 panic 不影响调度器
- 错误上报：返回 error 自动通过 `notify.SendErrMessage` 告警

---

## 13. 通用工具

> 参见 [common/defined/consts.go](common/defined/consts.go)

### 协程安全

使用 `coroutines.SafeGo` 启动协程，自动 panic 恢复：

```go
coroutines.SafeGo(ctx, func(ctx context.Context) {
    // 异步任务，panic 会被捕获并告警
})
```

### 日志上下文

`log.WithContext` 自动注入 RequestID 等链路信息：

```go
log.WithContext(ctx).
    WithField("order_id", orderID).
    Info("order created")
```

### 分页工具

`community.Paging` 提供标准分页结构，`community.PageOf` 构造响应：

```go
// Paging 字段：Page, PageSize, All, Total
// PageOf 用法：
return community.PageOf(list, req.Paging), nil
```

### 分布式锁

go-lib 提供三种分布式锁实现（`go-lib/dlock`）：

```go
provider := dlock.NewRedisProvider(redisClient)
mutex := provider.NewMutex("order:lock:" + orderID)
if err := mutex.Lock(ctx); err != nil { ... }
defer mutex.Unlock(ctx)
```

### 缓存

go-lib 提供泛型缓存（`go-lib/cache`），支持本地/Redis/二级缓存：

```go
c := cache.NewRedis[User](redisClient, cache.WithTTL(5*time.Minute))
user, err := c.GetOrLoad(ctx, key, func(ctx context.Context) (User, error) {
    return loadFromDB(ctx, id)
})
```
