package gormDB

import (
	"reflect"
	"strings"

	"github.com/Cotary/go-lib/common/community"
	"gorm.io/gorm"
)

// ApplyFilter 根据请求 DTO 上的 `filter` tag 自动构建一组 Scope。
//
// 在结构体字段上使用以下格式的 tag：
//
//	type ListReq struct {
//	    Name     string  `filter:"name,="`            // 默认 if 模式，零值跳过
//	    Status   *int64  `filter:"status,=,always"`   // nil 时折回零值，始终生效
//	    ParentID *int64  `filter:"parent_id,=,nullable"` // nil 时生成 IS NULL
//	    Keyword  string  `filter:"name|description,like"` // 多列 OR
//	    community.Paging   community.Paging
//	    community.Order    community.Order
//	}
//
// allowedOrder 传入 OrderWhitelist 的白名单 mapping。
//
// tag 格式：`filter:"col[,op[,mode]]"`
//   - col：数据库列名，多列以 | 分隔表示 OR
//   - op：=, <>, <, <=, >, >=, in, not in, like, not like（默认 =）
//   - mode：if（默认）/ always / nullable，分别对应 WhereIf / WhereAlways / WhereNullable
//
// 嵌套结构体会递归展开，指针类嵌套字段为 nil 时跳过。
//
// 性能说明：当前每次调用都通过反射遍历（profiling 发 collectFilterScopes 为热点时
// 可考虑 ApplyFilter(req) 返回的 Scopes 缓存到 sync.Map 中优化）。
func ApplyFilter(criteria any, allowedOrder ...map[string]string) Scope {
	return func(db *gorm.DB) *gorm.DB {
		var allowed map[string]string
		if len(allowedOrder) > 0 {
			allowed = allowedOrder[0]
		}
		for _, sc := range collectFilterScopes(criteria, allowed) {
			db = sc(db)
		}
		return db
	}
}

// collectFilterScopes 是 ApplyFilter 的内部实现，遍历结构体字段收集 Scope 列表。
// 按照 Where 条件 → community.Order 排序 → community.Paging 分页的顺序组装，确保生成的 SQL 子句顺序正确。
func collectFilterScopes(criteria any, allowedOrder map[string]string) []Scope {
	if criteria == nil {
		return nil
	}
	rv := reflect.ValueOf(criteria)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	var (
		whereScopes []Scope
		orderScope  Scope
		pagingScope Scope
	)
	walk(rv, allowedOrder, &whereScopes, &orderScope, &pagingScope)

	all := whereScopes
	if orderScope != nil {
		all = append(all, orderScope)
	}
	if pagingScope != nil {
		all = append(all, pagingScope)
	}
	return all
}

// walk 递归遍历 struct 字段，收集 Where/community.Order/community.Paging 三类 Scope
//   - 嵌套/匿名 struct 字段会递归展开
//   - 指针类型字段为 nil 时跳过
//   - 识别到 community.Paging / community.Order 类型时转为对应 Scope 而非当作 Where 条件
func walk(rv reflect.Value, allowed map[string]string, where *[]Scope, order *Scope, paging *Scope) {
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		fv := rv.Field(i)
		ft := rt.Field(i)
		if !fv.CanInterface() {
			continue
		}

		switch v := derefForWalk(fv).(type) {
		case community.Paging:
			if *paging == nil {
				p := v
				*paging = Paginate(&p)
			}
			continue
		case community.Order:
			if *order == nil && allowed != nil {
				*order = OrderWhitelist(v, allowed)
			}
			continue
		}

		if isStructLike(fv) {
			inner := fv
			if inner.Kind() == reflect.Ptr {
				if inner.IsNil() {
					continue
				}
				inner = inner.Elem()
			}
			walk(inner, allowed, where, order, paging)
			continue
		}

		tag := ft.Tag.Get("filter")
		if tag == "" {
			continue
		}
		if sc := buildScopeFromTag(tag, fv); sc != nil {
			*where = append(*where, sc)
		}
	}
}

// derefForWalk 对 community.Paging/community.Order 等值类型字段做解引用，使 type switch 能正确匹配。
// 仅用于类型识别，不影响后续 buildScopeFromTag 中对原始字段值的处理。
func derefForWalk(fv reflect.Value) any {
	if !fv.CanInterface() {
		return nil
	}
	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			return nil
		}
		return fv.Elem().Interface()
	}
	return fv.Interface()
}

// isStructLike 判断字段是否为需要递归展开的结构体（struct 或 *struct），排除 community.Paging/community.Order
func isStructLike(fv reflect.Value) bool {
	t := fv.Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	pagingT := reflect.TypeOf(community.Paging{})
	orderT := reflect.TypeOf(community.Order{})
	return t != pagingT && t != orderT
}

// buildScopeFromTag 根据单个字段的 tag 构建 Scope，tag 不合法时返回 nil（静默忽略）
func buildScopeFromTag(tag string, fv reflect.Value) Scope {
	parts := strings.Split(tag, ",")
	// 至少需要 col + op 两段，仅有一段时无法确定操作符，直接跳过。
	// 例如 `filter:"name"` 缺少 op，不会默认为 = 以避免隐式行为。
	if len(parts) < 2 {
		return nil
	}
	col := strings.TrimSpace(parts[0])
	if col == "" {
		return nil
	}
	op, ok := parseOp(parts[1])
	if !ok {
		return nil
	}
	mode := "if"
	if len(parts) >= 3 {
		mode = strings.ToLower(strings.TrimSpace(parts[2]))
	}

	val := fv.Interface()

	if strings.Contains(col, "|") {
		cols := strings.Split(col, "|")
		return multiColOR(cols, op, mode, val)
	}

	switch mode {
	case "always":
		return WhereAlways(col, op, val)
	case "nullable":
		return WhereNullable(col, op, val)
	default:
		return WhereIf(col, op, val)
	}
}

// multiColOR 将多个列名用同一操作符和值组合成 OR 条件，如 name|description 模式
func multiColOR(cols []string, op Op, mode string, val any) Scope {
	return func(db *gorm.DB) *gorm.DB {
		if IsNilPtr(val) {
			if mode != "always" && mode != "nullable" {
				return db
			}
		}
		v := Deref(val)
		if mode == "if" && IsZero(v) {
			return db
		}

		var orDB *gorm.DB
		for i, c := range cols {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			if i == 0 {
				orDB = applyOp(db.Session(&gorm.Session{NewDB: true}), c, op, v)
			} else {
				expr, args := applyOpExpr(c, op, v)
				if expr != "" {
					orDB = orDB.Or(expr, args...)
				}
			}
		}
		if orDB == nil {
			return db
		}
		return db.Where(orDB)
	}
}

// applyOpExpr 为 Or 子句构建 (clause, args...) 对，避免额外创建 Session 实例。
// 空切片的 IN / NOT IN 返回空字符串表示跳过。
func applyOpExpr(col string, op Op, val any) (string, []any) {
	switch op {
	case OpEq:
		return col + " = ?", []any{val}
	case OpNeq:
		return col + " <> ?", []any{val}
	case OpLt:
		return col + " < ?", []any{val}
	case OpLte:
		return col + " <= ?", []any{val}
	case OpGt:
		return col + " > ?", []any{val}
	case OpGte:
		return col + " >= ?", []any{val}
	case OpIn:
		if isEmptySlice(val) {
			return "", nil
		}
		return col + " IN ?", []any{val}
	case OpNotIn:
		if isEmptySlice(val) {
			return "", nil
		}
		return col + " NOT IN ?", []any{val}
	case OpLike:
		return col + " LIKE ?", []any{"%" + escapeLike(toString(val)) + "%"}
	case OpNotLike:
		return col + " NOT LIKE ?", []any{"%" + escapeLike(toString(val)) + "%"}
	}
	return col + " = ?", []any{val}
}

// parseOp 将 tag 中的操作符字符串解析为 Op 枚举，不识别的操作符返回 false
func parseOp(s string) (Op, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "=", "eq":
		return OpEq, true
	case "<>", "!=", "neq":
		return OpNeq, true
	case "<", "lt":
		return OpLt, true
	case "<=", "lte":
		return OpLte, true
	case ">", "gt":
		return OpGt, true
	case ">=", "gte":
		return OpGte, true
	case "in":
		return OpIn, true
	case "not in", "notin":
		return OpNotIn, true
	case "like":
		return OpLike, true
	case "not like", "notlike":
		return OpNotLike, true
	}
	return 0, false
}
