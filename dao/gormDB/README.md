# dao/pgsql — GORM 数据库封装

基于 [GORM](https://gorm.io/) 的数据库操作封装，支持 PostgreSQL、MySQL、SQLite。提供连接管理、事务、CRUD、查询构建、动态过滤、分页排序、Upsert 以及慢查询日志通知。

## 快速开始

### 初始化

```go
import "github.com/Cotary/go-lib/dao/gormDB"

config := &pgsql.GormConfig{
    Driver:      "postgres",                             // 支持: postgres, mysql, sqlite
    Dsn:         []string{"postgres://user:pass@host:5432/db?sslmode=disable"},
    ConnMaxLife: 3600,    // 连接最大生命周期(秒)
    ConnMaxIdle: 600,     // 空闲连接最大存活时间(秒)
    MaxOpens:    50,      // 最大打开连接数
    MaxIdles:    10,      // 最大空闲连接数
    LogLevel:    "info",  // silent / error / warn / info
    SlowThreshold: 500,   // 慢 SQL 阈值(ms)
    LogSaveDay:  30,      // 日志保留天数
}

// 推荐：返回 error 的方式
db, err := pgsql.NewGorm(config)
if err != nil {
    log.Fatal(err)
}

// 或在初始化阶段使用 Must 版本
db := pgsql.MustNewGorm(config)
```

### GormConfig 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `Driver` | string | 数据库驱动: `postgres`, `mysql`, `sqlite` |
| `Dsn` | []string | 数据源连接字符串 |
| `ConnMaxLife` | int | 连接最大生命周期(秒)，0=不限 |
| `ConnMaxIdle` | int | 空闲连接最大存活(秒)，0=不限 |
| `MaxOpens` | int | 最大打开连接数，0=不限 |
| `MaxIdles` | int | 最大空闲连接数，默认 2 |
| `LogLevel` | string | 日志级别: `silent`, `error`, `warn`, `info` |
| `SlowThreshold` | int64 | 慢查询阈值(毫秒)，默认 1000 |
| `LogSaveDay` | int64 | 日志保留天数，默认 30 |

---

## 基础 CRUD

### 模型定义

```go
type User struct {
    pgsql.BaseModel                              // 内嵌 ID, CreatedAt, UpdatedAt
    Name  string `gorm:"column:name"`
    Email string `gorm:"column:email;uniqueIndex"`
}

func (User) TableName() string { return "users" }
```

### 插入

```go
user := &User{Name: "张三", Email: "zhangsan@example.com"}
err := db.Insert(ctx, user)
```

### 查询

```go
// 单条查询（未找到返回 gorm.ErrRecordNotFound）
var user User
err := db.Get(ctx, &user, pgsql.Where("name = ?", "张三"))

// 单条查询（未找到返回 nil error）
err := db.MustGet(ctx, &user, pgsql.Where("name = ?", "张三"))

// 泛型查询
user, err := pgsql.Get[User](ctx, db, pgsql.Where("id = ?", 1))
user, err := pgsql.MustGet[User](ctx, db, pgsql.ID(1))

// 列表查询
users, err := pgsql.List[User](ctx, db, pgsql.Where("age > ?", 18))
```

### 更新

```go
// 指定字段更新
err := db.Update(ctx, &User{Name: "李四"}, []string{"name"}, pgsql.ID(1))

// 更新全部字段
err := db.Update(ctx, &user, []string{"*"}, pgsql.ID(1))
```

### 删除

```go
err := db.Delete(ctx, &User{}, pgsql.Where("id = ?", 1))
```

---

## 错误处理

```go
// DbErr: 将 RecordNotFound 和 RowsAffectedZero 视为 nil
err := pgsql.DbErr(someErr) // RecordNotFound → nil

// DbCheckErr: 返回 (是否存在, 数据库错误)
has, dbErr := pgsql.DbCheckErr(someErr)
if dbErr != nil { /* 数据库错误 */ }
if !has { /* 记录不存在 */ }
```

---

## QueryOption 模式

所有查询方法支持可变参数 `QueryOption`，可链式组合：

```go
users, err := pgsql.List[User](ctx, db,
    pgsql.Where("age > ?", 18),
    pgsql.Order("created_at desc"),
    pgsql.Limit(10),
    pgsql.Offset(20),
)
```

### 内置 QueryOption

| 函数 | 说明 |
|------|------|
| `Where(query, args...)` | 添加 WHERE 条件 |
| `ID(id)` | `WHERE id = ?` 的快捷方式 |
| `Select(fields...)` | 指定查询字段 |
| `Order(order)` | 指定排序（原始字符串） |
| `Limit(n)` | 限制返回条数 |
| `Offset(n)` | 偏移量 |
| `Table(name)` | 指定表名 |
| `Group(field)` | GROUP BY |
| `Having(query, args...)` | HAVING 条件 |
| `Joins(query, args...)` | JOIN 查询 |
| `Preload(column, conds...)` | 预加载关联 |
| `Distinct(fields...)` | DISTINCT 去重 |
| `Unscoped()` | 忽略软删除 |
| `Count(count)` | 计算总数 |
| `ForUpdate()` | 加 `FOR UPDATE` 行锁 |
| `ForShare()` | 加 `FOR SHARE` 共享锁 |
| `Pagination(paging)` | 自动计算 Total 并分页 |
| `Paging(paging)` | 仅分页，不计算 Total |
| `Total(count)` | 仅计算总数（清除 Limit/Offset） |
| `Orders(order, bindMaps)` | 动态排序（白名单模式） |

### 常用示例

```go
// Select + Group + Having: 聚合查询
type Result struct {
    Status string
    Count  int64
}
results, err := pgsql.List[Result](ctx, db,
    pgsql.Table("orders"),
    pgsql.Select("status", "COUNT(*) as count"),
    pgsql.Group("status"),
    pgsql.Having("COUNT(*) > ?", 5),
)

// Joins + Preload: 关联查询
users, err := pgsql.List[User](ctx, db,
    pgsql.Joins("LEFT JOIN orders ON orders.user_id = users.id"),
    pgsql.Preload("Orders"),
)

// Unscoped: 查询含软删除记录
all, err := pgsql.List[User](ctx, db, pgsql.Unscoped())

// Distinct: 去重
names, err := pgsql.List[User](ctx, db, pgsql.Distinct("name"))
```

### 自定义 QueryOption

```go
myOption := func(db *gorm.DB) *gorm.DB {
    return db.Where("status = ?", "active").Preload("Orders")
}
users, err := pgsql.List[User](ctx, db, myOption)
```

---

## 分页

```go
paging := &community.Paging{
    Page:     1,
    PageSize: 20,
}

// Pagination 自动计算 Total，然后分页
users, err := pgsql.List[User](ctx, db,
    pgsql.Where("status = ?", "active"),
    pgsql.Pagination(paging),  // 必须放在 Where 之后！
)
fmt.Printf("总数: %d, 当前页: %v\n", paging.Total, users)

// 如果 paging.All = true，则返回全部数据（仅计数不分页）
```

> **重要**: `Pagination` 必须放在所有 `Where` 条件之后，否则 COUNT 不包含后续条件。

---

## 排序

`Orders` 采用白名单模式，必须提供 `bindMaps` 声明允许排序的字段，未提供时不做任何排序。

```go
order := community.Order{
    OrderField: "created_at,name",
    OrderType:  "desc,asc",
}

bind := map[string]string{
    "created_at": "created_at {order_type}",  // {order_type} 会替换为 asc/desc
    "name":       "name",
}
opts := pgsql.Orders(order, bind)
```

说明：
- 只有 `bindMaps` 中声明的字段允许排序，前端传入未声明的字段直接忽略
- 模板中可使用 `{order_type}` 占位符，自动替换为 `asc`/`desc`
- **未提供 `bindMaps` 或 `bindMaps` 为空时，不做任何排序**（防止 SQL 注入）

---

## 事务

### 基本事务

```go
err := db.CtxTransaction(ctx, func(txCtx context.Context) error {
    user := &User{Name: "张三"}
    if err := db.Insert(txCtx, user); err != nil {
        return err  // 返回 error 自动回滚
    }

    order := &Order{UserID: user.ID, Amount: 100}
    if err := db.Insert(txCtx, order); err != nil {
        return err
    }

    return nil  // 返回 nil 自动提交
})
```

### 嵌套事务

```go
err := db.CtxTransaction(ctx, func(outerCtx context.Context) error {
    db.Insert(outerCtx, &User{Name: "外层"})

    // 嵌套事务使用 GORM SavePoint
    return db.CtxTransaction(outerCtx, func(innerCtx context.Context) error {
        return db.Insert(innerCtx, &Order{Amount: 50})
    })
})
```

### 事务选项

```go
txOpts := &sql.TxOptions{
    Isolation: sql.LevelReadCommitted,
    ReadOnly:  false,
}
err := db.CtxTransaction(ctx, fn, txOpts)
```

### WithContext

在事务内外统一获取可用的 DB 实例：

```go
// 事务外：返回普通 DB
// 事务内：返回当前事务的 tx
gormDB := db.WithContext(ctx)
```

---

## Save (Upsert)

根据冲突字段执行插入或更新：

```go
user := &User{Name: "张三", Email: "zs@example.com"}

// 冲突时更新指定字段
err := db.Save(ctx, user, []string{"name"}, []string{"email"})

// 冲突时更新全部字段
err := db.Save(ctx, user, []string{"*"}, []string{"email"})

// 冲突时不做任何操作
err := db.Save(ctx, user, nil, []string{"email"})
```

参数说明：
- `updateFields`: 冲突时更新哪些字段。`nil`/空=DoNothing；`["*"]`=全部更新；其他=指定字段
- `clausesFields`: 用于判断冲突的字段（通常是唯一索引列）

---

## QueryAndSave (先查后存)

在事务中先加锁查询，不存在则插入，存在则更新：

```go
user := &User{Name: "张三", Email: "zs@example.com", Age: 25}
condition := map[string]interface{}{"email": "zs@example.com"}

operation, err := db.QueryAndSave(ctx, user, []string{"name", "age"}, condition)
// operation: "insert" / "update" / "nothing"
```

工作流程：
1. `SELECT ... FOR UPDATE WHERE condition` 加锁查询
2. 未找到 → 执行 `Save`（Upsert），返回 `"insert"`
3. 已找到 → 执行 `Update`（排除 created_at），返回 `"update"` 或 `"nothing"`
4. 如果 `updateFields` 为 `nil`，找到后不更新，返回 `"nothing"`

---

## BuildQueryOptions (动态过滤)

通过结构体 tag 自动构建查询条件，适用于列表接口的筛选参数：

### 基本用法

```go
type UserFilter struct {
    Name   string `filter:"name,like"`
    Age    int    `filter:"age,gt"`
    Status string `filter:"status,eq"`
    community.Paging
    community.Order
}

filter := UserFilter{
    Name:   "张",         // 生成: name LIKE '%张%'
    Age:    18,           // 生成: age > 18
    Paging: community.Paging{Page: 1, PageSize: 20},
    Order:  community.Order{OrderField: "created_at", OrderType: "desc"},
}

// 传指针，确保 Pagination 的 Total 能回写
opts := pgsql.BuildQueryOptions(&filter, bindMap)
users, err := pgsql.List[User](ctx, db, opts...)
```

### filter tag 语法

格式: `filter:"column,operator"`

**比较操作符:**
- `eq` — 等于 (`=`)
- `ne` — 不等于 (`<>`)
- `gt` — 大于 (`>`)
- `lt` — 小于 (`<`)
- `ge` — 大于等于 (`>=`)
- `le` — 小于等于 (`<=`)

**模糊查询:**
- `like` — `LIKE '%value%'`
- `ilike` — `ILIKE '%value%'` (PostgreSQL 不区分大小写)

**集合操作:**
- `in` — `IN (?)`
- `not_in` — `NOT IN (?)`

**区间 (Between):**
- `bs` / `bs.<id>` — 区间起始值
- `be` / `be.<id>` — 区间结束值
- 配对使用生成 `BETWEEN ? AND ?`，只有一端则生成 `>=` 或 `<=`

```go
type TimeFilter struct {
    StartTime int64 `filter:"created_at,bs"`
    EndTime   int64 `filter:"created_at,be"`
    // 多组区间用 id 区分
    AmountMin float64 `filter:"amount,bs.amt"`
    AmountMax float64 `filter:"amount,be.amt"`
}
```

**组合操作符:**

用 `&` (AND) 和 `|` (OR) 连接多个操作符：
```go
RangeCheck int64 `filter:"created_at,ge&le"`  // created_at >= ? AND created_at <= ?
```

**多列查询:**

列名用 `|` 分隔：
```go
Search string `filter:"name|description,like"`  // (name LIKE ? OR description LIKE ?)
```

### 特殊字段

- `community.Paging` 类型 → 自动调用 `Pagination`
- `community.Order` 类型 → 自动调用 `Orders`
- 嵌套结构体 → 递归处理

### 零值跳过

零值字段自动跳过（空字符串、0、nil、空切片等），无需手动判断。

> **重要：传指针 vs 传值**
>
> `BuildQueryOptions` 的 `criteria` 参数**必须传指针**，否则 `Paging.Total` 无法回写到原始结构体。
> 传值时 Paging 会降级为仅分页（不计 Total）。
>
> ```go
> // 正确：传指针，Total 可回写
> opts := pgsql.BuildQueryOptions(&filter, bindMap)
>
> // 错误：传值，Total 永远为 0
> opts := pgsql.BuildQueryOptions(filter, bindMap)
> ```

---

## 日志与慢查询通知

初始化时自动配置 GORM Logger，支持：
- 文件日志（按天轮转）
- 慢查询检测（超过 `SlowThreshold` 自动记录 WARN）
- 慢查询外发通知（通过 `message.Sender` 接口）

```go
// 设置慢查询通知渠道
db.Logger.SetSender(mySender)
```

日志内容自动包含 RequestID、TransactionID、RequestURI 等上下文信息。

---

## 访问底层 GORM

```go
gormDB := db.DB()
gormDB.AutoMigrate(&User{})
```

> 注意：直接使用 `DB()` 操作会绕过事务上下文管理，建议优先使用 `WithContext(ctx)`。
