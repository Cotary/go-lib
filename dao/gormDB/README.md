# gormDB

`gormDB` 是 [GORM](https://gorm.io) 的精简增强层，遵循「**增强 GORM 而非替代 GORM**」的设计理念。

> 重要变更（破坏性升级）：本版本是对老 `gormDB` 的整体重写。老包基于 `QueryOption` 的封装方式已被替换为基于 GORM 原生 `Scopes` 的轻量增强。如果你正在从老版本迁移，请见文末「迁移指南」。

## 设计原则

1. **不造轮子**：能用 GORM 原生 API 表达的一律不封装
2. **增强而非替代**：用户依然直接写 GORM，仅在「有增量价值」的地方调用本包的 helper
3. **与 GORM Scopes 无缝对接**：所有条件辅助函数都是 `Scope = func(*gorm.DB) *gorm.DB` 类型，可直接传给 `db.Scopes(...)`

## Model 与 Scope 职责划分

Model 层负责「**这条 SQL 长什么样**」，Scope 负责「**这条 SQL 要不要加条件**」。

| 归属 | 内容 | 谁决定 |
| --- | --- | --- |
| **Model 层** | 表结构、关联、Preload、Joins、Select、Group、Having、Distinct、Unscoped、TableName | 数据模型设计者 |
| **Scope 层** | Where、Order、Limit、Offset、ForUpdate、Paginate | 业务调用方 |

**准则：** Scope 只做「加/减条件」，不改变查询的结构形状。如果一个操作改变了返回列、关联表或聚合方式，它属于 Model 层，应在 Model 方法中显式调用。

```go
// ✅ 正确：Model 层定义查询形状，Scope 层传入动态条件
func (m *UserModel) ListWithDept(ctx context.Context, scopes ...gormDB.Scope) ([]UserWithDept, error) {
    var out []UserWithDept
    db := m.g.WithContext(ctx).
        Model(&User{}).
        Select("users.*, departments.name as dept_name").
        Joins("LEFT JOIN departments ON departments.id = users.dept_id")
    db = db.Scopes(scopes...)
    return out, db.Find(&out).Error
}

// 调用方只关心条件，不关心 SQL 形状
list, err := userModel.ListWithDept(ctx,
    gormDB.WhereIf("users.status", gormDB.OpEq, req.Status),
    gormDB.OrderBy("users.id DESC"),
)

// ❌ 错误：把 Joins / Select 塞进 Scope，调用方无法一眼看出查询结构
func joinDept() gormDB.Scope {
    return func(db *gorm.DB) *gorm.DB {
        return db.Select("users.*, departments.name as dept_name").
            Joins("LEFT JOIN departments ON departments.id = users.dept_id")
    }
}
```

## 增量价值

| 关注点 | 提供的 API | 价值点 |
| --- | --- | --- |
| 零值/nil 跳过的动态 Where | `WhereIf` | HTTP 入参常见的「未填写就不过滤」语义，一行搞定 |
| 强制生效（含零值）的 Where | `WhereAlways` | `status=0` 这类需要参与查询的场景；nil 指针自动折回零值 |
| NULL 友好的 Where | `WhereNullable` / `IsNull` / `IsNotNull` | 区分 `IS NULL` 与 `= ''`，业务允许 NULL 列时使用 |
| 自动 count + Total 回写的分页 | `Paginate` | 同时回写 `Paging.Total`，无需额外变量 |
| 白名单驱动的排序 | `OrderWhitelist` | 防 SQL 注入，强制业务声明可排序列 |
| 内部固定排序透传 | `OrderBy` | 与 `db.Order(...)` 等价，仅用于风格统一 |
| Struct tag 一键装配整套过滤 | `ApplyFilter` | 把请求 DTO 整体翻译成 Scope 链 |
| 闭包式事务传播 | `CtxTransaction` / `WithContext` | 嵌套自动 SavePoint，业务代码无需手 commit |
| Upsert 与复合保存 | `Save` / `QueryAndSave` | 统一封装 ON CONFLICT 三种语义 |
| 透传式 Scope | `WhereRaw` / `Limit` / `Offset` / `ForUpdate` / `ForShare` | 让 `Scopes(...)` 链路风格统一，避免临时闭包 |
| 严格 Insert | `GormDrive.Insert` | 检查实际写入条数，被静默忽略时返回 `ErrRowsAffectedMismatch` |

> **结构化 GORM 操作（`Preload` / `Joins` / `Select` / `Group` / `Having` / `Distinct` / `Unscoped` 等）刻意不封装。**
> 它们属于 Model 层职责（见上方「Model 与 Scope 职责划分」），请在 Model 方法中显式调用，与 `Scopes(...)` 配合使用即可。

## 快速开始

```go
import "go-lib/dao/gormDB"

cfg := &gormDB.GormConfig{
    Driver: "mysql",
    Dsn:    []string{"user:pwd@tcp(127.0.0.1:3306)/demo?charset=utf8mb4"},
    LogDir: "./logs/gorm/",
}
g := gormDB.MustNewGorm(cfg)
```

`*GormDrive` 提供 3 个常用入口：

```go
g.WithContext(ctx)            // 业务最常用：自动接入事务上下文
g.DB()                        // 拿原生 *gorm.DB（不带事务自动注入）
g.CtxTransaction(ctx, fn)     // 闭包式事务，嵌套自动走 SavePoint
g.Health(ctx)                 // 探针
```

## Where 三种语义

```go
type ListReq struct {
    Name     string  // 来自 query
    Status   *int64  // 0 是合法值，需要可区分「未填写」
    ParentID *int64  // 数据库列允许 NULL
}

g.WithContext(ctx).Model(&User{}).Scopes(
    gormDB.WhereIf("name", gormDB.OpLike, req.Name),         // 空字符串 → 跳过
    gormDB.WhereAlways("status", gormDB.OpEq, req.Status),   // nil → 折回 0
    gormDB.WhereNullable("parent_id", gormDB.OpEq, req.ParentID), // nil → IS NULL
).Find(&out)
```

| 输入            | `WhereIf`             | `WhereAlways`          | `WhereNullable`       |
|-----------------|------------------------|-------------------------|------------------------|
| 非零值          | `col = val`            | `col = val`             | `col = val`            |
| 零值（非指针）  | 跳过                   | `col = 0`               | 跳过                   |
| nil 指针        | 跳过                   | `col = 0`（折回零值）   | `col IS NULL`          |
| nil 指针 + `OpNeq` | 跳过                | `col <> 0`              | `col IS NOT NULL`      |

## 分页

`Paging` 与 `community.Paging` 是同一类型，`Paginate` 会自动 count 并回写 `p.Total`：

```go
list, err := gormDB.PageList[User](ctx, g, &req.Paging,
    gormDB.WhereIf("name", gormDB.OpLike, req.Name),
    gormDB.OrderBy("id DESC"),
)
return community.PageOf(list, req.Paging), err
```

`Paginate` 与位置无关：内部用 `db.Session(&gorm.Session{}).Limit(-1).Offset(-1)` 克隆做 Count，无论 `Paginate` 在 `Scopes` 链中什么位置都能正确统计总数。

## 排序

```go
// 1) 内部固定排序：直接透传，等价 db.Order("id DESC")
gormDB.OrderBy("id DESC")

// 2) 用户输入排序：必须走白名单防注入
gormDB.OrderWhitelist(req.Order, map[string]string{
    "created_at": "u.created_at",
    "name":       "u.name",
})
```

白名单 mapping 的 value 是 SQL 表达式，可携带表别名 / 函数；key 不在表中或 `OrderField` 为空时不加 `ORDER BY`。

## 一键装配：`ApplyFilter`

把整个请求 DTO 上的 `filter` tag 翻译成完整的 Scope 链：

```go
type ListReq struct {
    Name     string `filter:"name,="`
    Age      int64  `filter:"age,gte"`
    Status   *int64 `filter:"status,=,always"`
    ParentID *int64 `filter:"parent_id,=,nullable"`
    Keyword  string `filter:"name|description,like"` // 多列 OR
    community.Paging
    community.Order
}

allowed := map[string]string{"created_at": "u.created_at"}
list, err := gormDB.PageList[User](ctx, g, &req.Paging,
    gormDB.ApplyFilter(req, allowed),
)
```

tag 语法：`filter:"col[,op[,mode]]"`

| 段 | 取值 | 缺省 |
| --- | --- | --- |
| col | 列名；多列以 `\|` 分隔表示 OR | 必填 |
| op | `=` / `<>` / `<` / `<=` / `>` / `>=` / `in` / `not in` / `like` / `not like`（也接受 `eq` / `neq` / `lt` / `lte` / `gt` / `gte` / `notin` / `notlike` 别名） | 必填，未指定时 tag 整体被忽略 |
| mode | `if`（默认）/ `always` / `nullable`，对应 `WhereIf` / `WhereAlways` / `WhereNullable` | `if` |

嵌套结构体字段会被递归展开；指针型嵌套字段为 nil 时跳过；同名 `Paging` / `Order` 字段会被自动识别为分页/排序而非 Where 条件。

## CRUD 与 Upsert

```go
// 严格插入：实际写入数 ≠ 传入条数（被静默忽略）时返回 ErrRowsAffectedMismatch。
err := g.Insert(ctx, []User{u1, u2})

// 按条件更新指定字段
err := g.Update(ctx, &user, []string{"name", "age"},
    gormDB.WhereIf("id", gormDB.OpEq, user.ID))

// Upsert：fields 控制冲突时的更新行为
g.Save(ctx, &user, nil,           "id")            // DoNothing
g.Save(ctx, &user, []string{"*"}, "id")            // UpdateAll
g.Save(ctx, &user, []string{"name"}, "id")         // 仅更新 name

// 锁查 → 不存在则 Upsert，存在则 Update（事务自动复用外层 ctx 中的事务）
op, err := g.QueryAndSave(ctx, &user, []string{"name"},
    map[string]any{"email": user.Email})
// op = OperationInsert / OperationUpdate / OperationNothing
```

## 事务

```go
err := g.CtxTransaction(ctx, func(ctx context.Context) error {
    // ctx 已挂载 *gorm.DB，WithContext 自动取出
    if err := g.Insert(ctx, &order); err != nil {
        return err
    }
    // 嵌套调用：内层走 SavePoint
    return g.CtxTransaction(ctx, func(ctx context.Context) error {
        return g.Update(ctx, &stock, []string{"qty"},
            gormDB.WhereIf("id", gormDB.OpEq, stock.ID))
    })
})
```

闭包返回 `error` 时自动 Rollback，否则 Commit；外层与内层共享同一个 `TransactionID`，日志可串联。

## 错误辅助

| Helper | 用途 |
| --- | --- |
| `DbErr(err)` | 把「预期内的空结果」（`gorm.ErrRecordNotFound` / `RowsAffectedZero`）归一为 nil |
| `DbCheckErr(err)` | `(has bool, dbErr error)` 模式，明确区分「未命中」与「真实错误」 |
| `DbAffectedErr(db)` | 执行成功但 `RowsAffected==0` 时返回 `RowsAffectedZero` |
| `errors.Is(err, ErrRowsAffectedMismatch)` | 判断 `Insert` 实际写入数与传入数不一致 |

## 透传 Scope

仅为风格统一存在，对应 GORM 原生方法：

```go
gormDB.WhereRaw("status = ? AND tenant_id = ?", 1, tid)
gormDB.Limit(50)
gormDB.Offset(100)
gormDB.ForUpdate()
gormDB.ForShare()
```

## 文件结构

```
dao/gormDB/
├── doc.go        // 包文档
├── driver.go     // GormConfig / GormDrive / NewGorm / Health
├── log.go        // GORM logger 适配 + 慢 SQL 推送
├── tx.go         // CtxTransaction / WithContext
├── func.go       // CRUD / Save / QueryAndSave / 错误辅助 / 泛型 Get/List/PageList
├── query.go      // Scope / Op / 零值工具 / Where 系列 / Paginate / Order / 透传 Scope
└── filter.go     // ApplyFilter（基于 struct tag）
```

