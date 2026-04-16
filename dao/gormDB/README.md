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

## FilterBuilder 链式过滤

`Filter()` 返回链式构造器 `FilterBuilder`，通过 `.Eq()` / `.Gt()` 等方法逐条添加过滤条件，最终 `.Build()` 生成 `[]QueryOption`。适合在业务代码中根据请求参数灵活组装查询条件。

### 基本用法

```go
opts := gormDB.Filter().
    Eq("status", req.Status).
    ILike("name", req.Name).
    Gte("created_at", req.StartTime).
    Lte("created_at", req.EndTime).
    Option(gormDB.Pagination(paging)).
    Option(gormDB.Order("id DESC")).
    Build()

users, err := gormDB.List[User](ctx, db, opts...)
```

### 过滤方法

| 方法 | SQL 效果 | 说明 |
|------|----------|------|
| `Eq(col, val)` | `col = val` | 等值匹配 |
| `Neq(col, val)` | `col <> val` | 不等于 |
| `Gt(col, val)` | `col > val` | 大于 |
| `Lt(col, val)` | `col < val` | 小于 |
| `Gte(col, val)` | `col >= val` | 大于等于 |
| `Lte(col, val)` | `col <= val` | 小于等于 |
| `Like(col, val)` | `col LIKE '%val%'` | 模糊匹配 |
| `ILike(col, val)` | `col ILIKE '%val%'` | 大小写不敏感模糊匹配（PostgreSQL） |
| `In(col, val)` | `col IN (?)` | 集合包含，val 为切片 |
| `NotIn(col, val)` | `col NOT IN (?)` | 集合排除，val 为切片 |
| `MustEq(col, val)` | `col = val` | 强制等值，**不做零值检查**，仅 nil 跳过 |
| `Must(col, val)` | `col = val` | `MustEq` 的别名，向后兼容 |
| `MustNeq(col, val)` | `col <> val` | 强制不等于，不做零值检查 |
| `MustGt(col, val)` | `col > val` | 强制大于，不做零值检查 |
| `MustLt(col, val)` | `col < val` | 强制小于，不做零值检查 |
| `MustGte(col, val)` | `col >= val` | 强制大于等于，不做零值检查 |
| `MustLte(col, val)` | `col <= val` | 强制小于等于，不做零值检查 |
| `MustLike(col, val)` | `col LIKE '%val%'` | 强制模糊匹配，不做零值检查 |
| `MustILike(col, val)` | `col ILIKE '%val%'` | 强制不敏感模糊匹配，不做零值检查 |
| `MustIn(col, val)` | `col IN (?)` | 强制集合包含，不做零值检查 |
| `MustNotIn(col, val)` | `col NOT IN (?)` | 强制集合排除，不做零值检查 |

### 零值与指针的自动跳过规则

所有过滤方法（`Must` 除外）均内置零值/nil 自动跳过，无需手动 `if` 判断：

**非指针类型** — 零值跳过，非零值加条件：

```go
gormDB.Filter().
    Eq("status", int64(0)).   // 零值 → 跳过，不生成 WHERE
    Eq("status", int64(1)).   // 非零值 → 生成 WHERE status = 1
    Eq("name", "").           // 空字符串 → 跳过
    Eq("name", "alice").      // 非空 → 生成 WHERE name = 'alice'
    Build()
```

**指针类型** — nil 跳过，非 nil 即使指向零值也加条件：

```go
var nilPtr *int64             // nil 指针
zero := int64(0)
zeroPtr := &zero              // 非 nil，但值为 0

gormDB.Filter().
    Eq("status", nilPtr).     // nil → 跳过，不生成 WHERE
    Eq("status", zeroPtr).    // 非 nil → 生成 WHERE status = 0
    Build()
```

> **指针的典型场景：** 当业务上需要区分「用户没传该参数」和「用户显式传了 0 / 空串」时，
> 请求结构体中使用指针字段（如 `Status *int64`），这样 nil 表示未传（跳过），
> `*int64(0)` 表示用户确实要查 status=0 的记录。

**MustXxx 系列** — 不做零值检查，仅 nil 跳过：

所有操作符都有对应的 Must 版本，用于非指针零值也是有效业务值的场景：

```go
gormDB.Filter().
    MustEq("mchid", int64(0)).    // 0 不是 nil → 生成 WHERE mchid = 0
    MustGt("score", int64(0)).    // 0 不是 nil → 生成 WHERE score > 0
    MustLte("age", int64(0)).     // 0 不是 nil → 生成 WHERE age <= 0
    MustEq("mchid", nil).         // nil → 跳过
    Build()
```

> **Xxx vs MustXxx 选择原则：** 默认使用 `Eq`/`Gt`/`Lte` 等普通方法（零值自动跳过更安全）；
> 仅当确定"零值也是有意义的查询条件"时才使用 `MustXxx` 版本。

### 注入自定义 QueryOption

`Option` 和 `Options` 可将任意 `QueryOption`（分页、排序、自定义条件等）混入 FilterBuilder：

```go
opts := gormDB.Filter().
    Eq("status", req.Status).
    Option(gormDB.Pagination(paging)).       // 单个注入，nil 安全
    Options(gormDB.Order("id DESC"), gormDB.Where("age > ?", 18)). // 批量注入，自动过滤 nil
    Build()
```

### 区间查询

使用 `Gte` + `Lte` 组合实现区间过滤：

```go
gormDB.Filter().
    Gte("created_at", req.StartTime).  // 零值自动跳过
    Lte("created_at", req.EndTime).
    Build()
```

---

## 独立 Filter 函数

除了链式构造器，还提供独立的 Filter 函数，每个返回单个 `QueryOption`，可直接用于 `List` / `Get` 等方法的可变参数：

| 函数 | 等价的 FilterBuilder 方法 |
|------|--------------------------|
| `FilterEq(col, val)` | `Filter().Eq(col, val)` |
| `FilterNeq(col, val)` | `Filter().Neq(col, val)` |
| `FilterGt(col, val)` | `Filter().Gt(col, val)` |
| `FilterLt(col, val)` | `Filter().Lt(col, val)` |
| `FilterGte(col, val)` | `Filter().Gte(col, val)` |
| `FilterLte(col, val)` | `Filter().Lte(col, val)` |
| `FilterLike(col, val)` | `Filter().Like(col, val)` |
| `FilterILike(col, val)` | `Filter().ILike(col, val)` |
| `FilterIn(col, val)` | `Filter().In(col, val)` |
| `FilterNotIn(col, val)` | `Filter().NotIn(col, val)` |

零值/nil 跳过规则与 FilterBuilder 方法完全一致。

```go
users, err := gormDB.List[User](ctx, db,
    gormDB.FilterEq("status", req.Status),
    gormDB.FilterLike("name", req.Name),
    gormDB.FilterGte("created_at", req.StartTime),
    gormDB.Order("id DESC"),
    gormDB.Pagination(paging),
)
```

> **如何选择：** 条件较多时推荐 `Filter()` 链式构造器，可读性更好；
> 条件只有 1-2 个时用独立函数更简洁。两者可以混用。

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

**比较操作符（优先使用 SQL 符号，也接受别名）:**
- `=` / `eq` — 等于
- `<>` / `ne` — 不等于
- `>` / `gt` — 大于
- `<` / `lt` — 小于
- `>=` / `gte` — 大于等于
- `<=` / `lte` — 小于等于

**模糊查询:**
- `like` — `LIKE '%value%'`
- `ilike` — `ILIKE '%value%'` (PostgreSQL 不区分大小写)

**集合操作:**
- `in` — `IN (?)`
- `not_in` — `NOT IN (?)`

**多列查询:**

列名用 `|` 分隔，多个列之间为 OR 关系：
```go
Search string `filter:"name|description,like"`  // (name LIKE ? OR description LIKE ?)
```

**区间查询替代方案:**

如需区间过滤，使用两个独立字段分别配合 `gte` 和 `lte`：
```go
type TimeFilter struct {
    StartTime int64 `filter:"created_at,gte"`   // created_at >= ?
    EndTime   int64 `filter:"created_at,lte"`   // created_at <= ?
}
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
