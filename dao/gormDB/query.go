package gormDB

import (
	"reflect"
	"strings"

	"github.com/Cotary/go-lib/common/community"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Scope 是 GORM 原生的 scope 函数类型别名，方便包内统一引用。
//
// 本包的 Where* / Paginate / OrderXxx 等函数均返回 Scope 类型，
// 可直接传给 db.Scopes(s1, s2, ...) 链式调用。
type Scope = func(*gorm.DB) *gorm.DB

// ===== 操作符定义 =====

// Op 是查询条件的 SQL 操作符枚举，使用 int 类型而非字符串以避免拼写错误
type Op int

const (
	OpEq Op = iota
	OpNeq
	OpLt
	OpLte
	OpGt
	OpGte
	OpIn
	OpNotIn
	OpLike
	OpNotLike
)

// String 返回调试 / 日志用途的可读 SQL 操作符字符串
func (o Op) String() string {
	switch o {
	case OpEq:
		return "="
	case OpNeq:
		return "<>"
	case OpLt:
		return "<"
	case OpLte:
		return "<="
	case OpGt:
		return ">"
	case OpGte:
		return ">="
	case OpIn:
		return "IN"
	case OpNotIn:
		return "NOT IN"
	case OpLike:
		return "LIKE"
	case OpNotLike:
		return "NOT LIKE"
	default:
		return "?"
	}
}

// isEmptySlice 判断值是否为空切片/数组，用于防止 IN () 生成非法 SQL
func isEmptySlice(v any) bool {
	rv := reflect.ValueOf(v)
	return (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) && rv.Len() == 0
}

// escapeLike 转义 LIKE 操作中的 SQL 通配符（%、_、\），防止用户输入被意外解释为模式匹配
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// applyOp 将 (col, op, val) 组合成 GORM 的 Where 子句
//
// LIKE / NOT LIKE 会自动包裹 % 并转义用户输入中的通配符，IN / NOT IN 要求 val 为 slice。
// 空切片的 IN / NOT IN 不生成条件（防止 SQL 语法错误），无法识别的 op 直接返回原始 db。
func applyOp(db *gorm.DB, col string, op Op, val any) *gorm.DB {
	switch op {
	case OpEq:
		return db.Where(col+" = ?", val)
	case OpNeq:
		return db.Where(col+" <> ?", val)
	case OpLt:
		return db.Where(col+" < ?", val)
	case OpLte:
		return db.Where(col+" <= ?", val)
	case OpGt:
		return db.Where(col+" > ?", val)
	case OpGte:
		return db.Where(col+" >= ?", val)
	case OpIn:
		if isEmptySlice(val) {
			return db
		}
		return db.Where(col+" IN ?", val)
	case OpNotIn:
		if isEmptySlice(val) {
			return db
		}
		return db.Where(col+" NOT IN ?", val)
	case OpLike:
		return db.Where(col+" LIKE ?", "%"+escapeLike(toString(val))+"%")
	case OpNotLike:
		return db.Where(col+" NOT LIKE ?", "%"+escapeLike(toString(val))+"%")
	}
	return db
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ===== 零值 / nil 工具 =====

// IsZeroer 是自定义零值判断接口，类型实现此接口可覆盖默认零值检测。
// 典型用例：decimal.Decimal 等自定义数值类型、Money 等业务封装类型。
type IsZeroer interface {
	IsZero() bool
}

// IsZero 判断值是否为零值，依次检查：1) nil 指针 2) IsZeroer 接口 3) reflect.Value.IsZero
func IsZero(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return isZeroValue(rv)
}

// isZeroValue 是 IsZero 的内部实现，递归处理指针解引用后判断零值。
// 独立为函数是因为 ApplyFilter 中需要直接传入 reflect.Value 调用。
func isZeroValue(rv reflect.Value) bool {
	if !rv.IsValid() {
		return true
	}
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return true
		}
		return isZeroValue(rv.Elem())
	}
	if rv.CanInterface() {
		if z, ok := rv.Interface().(IsZeroer); ok {
			return z.IsZero()
		}
	}
	return rv.IsZero()
}

// IsNilPtr 判断值是否为 nil 指针，非指针类型始终返回 false
func IsNilPtr(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Ptr && rv.IsNil()
}

// Deref 递归解引用指针直到获得底层值，供 Where 等函数获取实际的条件值。
// nil 指针返回 nil。
func Deref(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.CanInterface() {
		return rv.Interface()
	}
	return v
}

// zeroOf 返回指定值类型的零值实例，供 WhereAlways 将 nil 指针折回为零值
func zeroOf(v any) any {
	if v == nil {
		return nil
	}
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return reflect.Zero(t).Interface()
}

// ===== Where 构建 =====

// WhereIf 零值/nil 时跳过，非零值时生效
//   - val 为 nil 指针 → 跳过
//   - val 为零值 → 跳过
//   - 其他 → 生成条件
//
// 适用于 HTTP/RPC 请求参数中「未填写则不过滤」的场景
func WhereIf(col string, op Op, val any) Scope {
	return func(db *gorm.DB) *gorm.DB {
		if IsNilPtr(val) {
			return db
		}
		v := Deref(val)
		if IsZero(v) {
			return db
		}
		return applyOp(db, col, op, v)
	}
}

// WhereAlways 无论值是否为零值都强制生效
//   - val 为 nil 指针 → 折回为类型零值后仍生成条件
//   - 其他 → 直接使用实际值生成条件
//
// 适用于零值本身有业务含义的场景，例如 status=0 表示特定状态，
// 需要 WHERE status = 0 而非被忽略的情况。
func WhereAlways(col string, op Op, val any) Scope {
	return func(db *gorm.DB) *gorm.DB {
		if IsNilPtr(val) {
			return applyOp(db, col, op, zeroOf(val))
		}
		return applyOp(db, col, op, Deref(val))
	}
}

// WhereNullable 支持 NULL 语义的条件构建
//   - val 为 nil 指针 → 生成 IS NULL（或 IS NOT NULL，当 op=OpNeq 时）
//   - val 为零值 → 跳过
//   - 其他 → 生成正常条件
//
// 适用于数据库列允许 NULL 且业务需要区分 NULL 与空值的场景
func WhereNullable(col string, op Op, val any) Scope {
	return func(db *gorm.DB) *gorm.DB {
		if IsNilPtr(val) {
			if op == OpNeq {
				return db.Where(col + " IS NOT NULL")
			}
			return db.Where(col + " IS NULL")
		}
		v := Deref(val)
		if IsZero(v) {
			return db
		}
		return applyOp(db, col, op, v)
	}
}

// IsNull 生成 col IS NULL 条件，适用于软删除 / 父节点为空等场景
func IsNull(col string) Scope {
	return func(db *gorm.DB) *gorm.DB { return db.Where(col + " IS NULL") }
}

// IsNotNull 生成 col IS NOT NULL 条件
func IsNotNull(col string) Scope {
	return func(db *gorm.DB) *gorm.DB { return db.Where(col + " IS NOT NULL") }
}

// ===== 分页 =====

// Paginate 执行 count 并回写 Total 到 *p，再根据 Page/PageSize 生成 Limit/Offset。
//
// 位置无关性：内部使用 db.Session 分离独立 statement，先用 Limit(-1).Offset(-1) 清除
// 已有的分页设置后做 Count，因此 Paginate 在 Scopes 链中的位置不影响统计结果。
//
// p.All == true 时仅统计 Total 而不分页（返回全部数据）。
// p 为 nil 时为 no-op，不会产生任何分页或计数行为。
func Paginate(p *community.Paging) Scope {
	return func(db *gorm.DB) *gorm.DB {
		if p == nil {
			return db
		}

		// Session{} 分离一个新 Statement 用于独立的 Count 查询，不会干扰原始查询的 community.Order/Limit
		var total int64
		countDB := db.Session(&gorm.Session{}).Limit(-1).Offset(-1)
		countDB.Count(&total)
		p.Total = total

		if p.All {
			return db
		}
		page := p.Page
		if page < 1 {
			page = 1
		}
		size := p.PageSize
		if size <= 0 {
			size = 20
		}
		return db.Limit(size).Offset((page - 1) * size)
	}
}

// PaginateNoCount 仅执行分页（Limit/Offset），不执行 Count 查询。
// 适用于无限滚动、瀑布流等不需要 Total 的场景，相比 Paginate 少一次 COUNT 查询。
// p 为 nil 时为 no-op。
func PaginateNoCount(p *community.Paging) Scope {
	return func(db *gorm.DB) *gorm.DB {
		if p == nil {
			return db
		}
		if p.All {
			return db
		}
		page := p.Page
		if page < 1 {
			page = 1
		}
		size := p.PageSize
		if size <= 0 {
			size = 20
		}
		return db.Limit(size).Offset((page - 1) * size)
	}
}

// ===== 排序 =====

// OrderWhitelist 以白名单方式安全地构建 ORDER BY，根据 o.OrderField/OrderType 和 mapping 生成排序子句。
//
// mapping[key] = sqlExpr，如 {"created_at":"t.created_at"}。
// key 不在 mapping 中或 OrderField 为空时不生成 ORDER BY。
// OrderType 仅接受 "asc"/"desc"（不区分大小写），其他值默认为 ASC。
//
// 通过白名单机制防止用户输入恶意列名导致 SQL 注入。
func OrderWhitelist(o community.Order, mapping map[string]string) Scope {
	return func(db *gorm.DB) *gorm.DB {
		if o.OrderField == "" {
			return db
		}
		expr, ok := mapping[o.OrderField]
		if !ok {
			return db
		}
		typ := strings.ToUpper(o.OrderType)
		if typ != "ASC" && typ != "DESC" {
			typ = "ASC"
		}
		return db.Order(expr + " " + typ)
	}
}

// Order 直接透传 ORDER BY 表达式，等价于 GORM 原生的 db.community.Order(expr)。
//
// 仅用于内部固定排序场景，用户可控的排序请使用 OrderWhitelist 防注入。
func Order(expr string) Scope {
	return func(db *gorm.DB) *gorm.DB {
		if strings.TrimSpace(expr) == "" {
			return db
		}
		return db.Order(expr)
	}
}

// ===== 透传 Scope =====
//
// 以下是对 GORM 原生方法的薄封装，仅为让 Scopes(...) 链路风格统一。
// 避免在 Scopes 调用中混入 func(db *gorm.DB) *gorm.DB { return ... } 匿名闭包。
//
// 复杂的 GORM 操作（Preload / Joins / Select / Group / Having / Distinct / Unscoped 等）
// 不在此封装。它们应在 Model 层显式调用，与 Scopes 配合使用以保持查询意图清晰。

// WhereRaw 透传 GORM 的 Where，支持占位符/named 参数等原生语法
func WhereRaw(query any, args ...any) Scope {
	return func(db *gorm.DB) *gorm.DB { return db.Where(query, args...) }
}

// Limit 透传 GORM 的 Limit，适用于不需要 Paginate 计数的简单限制场景
func Limit(n int) Scope {
	return func(db *gorm.DB) *gorm.DB { return db.Limit(n) }
}

// Offset 透传 GORM 的 Offset，适用于不需要 Paginate 计数的简单偏移场景
func Offset(n int) Scope {
	return func(db *gorm.DB) *gorm.DB { return db.Offset(n) }
}

// ID 主键查询 helper，等价 db.Where("id = ?", id)
func ID(id int64) Scope {
	return func(db *gorm.DB) *gorm.DB { return db.Where("id = ?", id) }
}

// Unscoped 透传 db.Unscoped()，绕过软删除过滤
func Unscoped() Scope {
	return func(db *gorm.DB) *gorm.DB { return db.Unscoped() }
}

// ForUpdate 添加 SELECT ... FOR UPDATE 行锁子句
func ForUpdate() Scope {
	return func(db *gorm.DB) *gorm.DB {
		return db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
}

// ForShare 添加 SELECT ... FOR SHARE 共享锁子句
func ForShare() Scope {
	return func(db *gorm.DB) *gorm.DB {
		return db.Clauses(clause.Locking{Strength: "SHARE"})
	}
}
