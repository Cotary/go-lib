package gormDB

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/Cotary/go-lib/common/community"

	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// 零值判断 & SQL 子句构建（FilterBuilder 与 BuildQueryOptions 共用）
// ---------------------------------------------------------------------------

var (
	typCommunityPaging    = reflect.TypeOf(community.Paging{})
	typCommunityPagingPtr = reflect.PtrTo(typCommunityPaging)
	typCommunityOrder     = reflect.TypeOf(community.Order{})
	typCommunityOrderPtr  = reflect.PtrTo(typCommunityOrder)
)

type isZeroer interface {
	IsZero() bool
}

func isZeroValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}

	if v.CanInterface() {
		if z, ok := v.Interface().(isZeroer); ok {
			return z.IsZero()
		}
	}

	switch v.Kind() {
	case reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Slice, reflect.Array, reflect.Map:
		return v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !isZeroValue(v.Field(i)) {
				return false
			}
		}
		return true
	}
	return false
}

func isZeroInterface(val interface{}) bool {
	if val == nil {
		return true
	}
	return isZeroValue(reflect.ValueOf(val))
}

func splitFilterColumns(colExpr string) []string {
	parts := strings.Split(colExpr, "|")
	var cols []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			cols = append(cols, p)
		}
	}
	return cols
}

var opSymMap = map[string]string{
	"eq": "=", "ne": "<>",
	"gt": ">", "lt": "<",
	"ge": ">=", "le": "<=",
}

func buildFilterClause(colExpr, op string, val interface{}) (string, []interface{}) {
	op = strings.TrimSpace(op)
	if op == "" || strings.ContainsAny(op, "&|") {
		return "", nil
	}
	cols := splitFilterColumns(colExpr)
	if len(cols) == 0 {
		return "", nil
	}

	var colClauses []string
	var args []interface{}

	switch op {
	case "eq", "ne", "gt", "lt", "ge", "le", "=", "<>", ">", "<", ">=", "<=":
		sym, ok := opSymMap[op]
		if !ok {
			sym = op
		}
		for _, col := range cols {
			colClauses = append(colClauses, fmt.Sprintf("%s %s ?", col, sym))
			args = append(args, val)
		}

	case "like", "ilike":
		pat := fmt.Sprintf("%%%v%%", val)
		for _, col := range cols {
			colClauses = append(colClauses, fmt.Sprintf("%s %s ?", col, op))
			args = append(args, pat)
		}

	case "in", "not_in":
		kw := "IN"
		if op == "not_in" {
			kw = "NOT IN"
		}
		for _, col := range cols {
			colClauses = append(colClauses, fmt.Sprintf("%s %s ?", col, kw))
			args = append(args, val)
		}

	default:
		return "", nil
	}

	if len(colClauses) == 1 {
		return colClauses[0], args
	}
	return "(" + strings.Join(colClauses, " OR ") + ")", args
}

func noopQueryOption() QueryOption {
	return func(db *gorm.DB) *gorm.DB { return db }
}

// filterQueryOption 统一的过滤条件入口：nil / 零值跳过；非法 op 跳过。
func filterQueryOption(colExpr, op string, val interface{}) QueryOption {
	if isZeroInterface(val) {
		return noopQueryOption()
	}
	cl, args := buildFilterClause(colExpr, op, val)
	if cl == "" {
		return noopQueryOption()
	}
	return Where(cl, args...)
}

// ---------------------------------------------------------------------------
// 独立 Filter 函数（不依赖 FilterBuilder，可单独使用）
// ---------------------------------------------------------------------------

func FilterEq(val interface{}, col string) QueryOption   { return filterQueryOption(col, "eq", val) }
func FilterNe(val interface{}, col string) QueryOption   { return filterQueryOption(col, "ne", val) }
func FilterGt(val interface{}, col string) QueryOption   { return filterQueryOption(col, "gt", val) }
func FilterLt(val interface{}, col string) QueryOption   { return filterQueryOption(col, "lt", val) }
func FilterGe(val interface{}, col string) QueryOption   { return filterQueryOption(col, "ge", val) }
func FilterLe(val interface{}, col string) QueryOption   { return filterQueryOption(col, "le", val) }
func FilterLike(val interface{}, col string) QueryOption { return filterQueryOption(col, "like", val) }
func FilterILike(val interface{}, col string) QueryOption {
	return filterQueryOption(col, "ilike", val)
}
func FilterIn(val interface{}, col string) QueryOption { return filterQueryOption(col, "in", val) }
func FilterNotIn(val interface{}, col string) QueryOption {
	return filterQueryOption(col, "not_in", val)
}

// ---------------------------------------------------------------------------
// BuildQueryOptions — 结构体 filter tag 反射模式
// ---------------------------------------------------------------------------

type queryOptionBuckets struct {
	where  []QueryOption
	paging []QueryOption
	order  []QueryOption
}

func (b *queryOptionBuckets) appendAll(src queryOptionBuckets) {
	b.where = append(b.where, src.where...)
	b.paging = append(b.paging, src.paging...)
	b.order = append(b.order, src.order...)
}

func splitOrderFilterTag(tag string) (spec string, ok bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", false
	}
	suffixes := []string{",order", ", order"}
	for _, suf := range suffixes {
		if strings.HasSuffix(tag, suf) {
			spec = strings.TrimSpace(strings.TrimSuffix(tag, suf))
			return spec, true
		}
	}
	return "", false
}

func parseOrderBindSpec(spec string) map[string]string {
	out := make(map[string]string)
	for _, raw := range strings.Split(spec, ";") {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}
		key, tpl, hasColon := strings.Cut(part, ":")
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if hasColon {
			tpl = strings.TrimSpace(tpl)
			if tpl != "" {
				out[key] = tpl
			}
			continue
		}
		out[key] = key + " " + orderType
	}
	return out
}

func orderBindMapForField(ft reflect.StructField, bindOrders []map[string]string) map[string]string {
	tag := strings.TrimSpace(ft.Tag.Get("filter"))
	spec, isOrderTag := splitOrderFilterTag(tag)
	if !isOrderTag {
		if len(bindOrders) > 0 && bindOrders[0] != nil {
			return bindOrders[0]
		}
		return nil
	}
	if spec == "" {
		if len(bindOrders) > 0 && bindOrders[0] != nil {
			return bindOrders[0]
		}
		return nil
	}
	m := parseOrderBindSpec(spec)
	if len(m) == 0 {
		if len(bindOrders) > 0 && bindOrders[0] != nil {
			return bindOrders[0]
		}
		return nil
	}
	return m
}

func collectQueryOptionBuckets(criteria interface{}, bindOrders ...map[string]string) queryOptionBuckets {
	var b queryOptionBuckets

	if criteria == nil {
		return b
	}
	rv := reflect.ValueOf(criteria)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return b
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return b
	}
	rt := rv.Type()

	for i := 0; i < rt.NumField(); i++ {
		ft := rt.Field(i)
		fv := rv.Field(i)

		switch ft.Type {
		case typCommunityPagingPtr:
			if fv.IsNil() {
				continue
			}
			p := fv.Interface().(*community.Paging)
			b.paging = append(b.paging, Pagination(p))
			continue
		case typCommunityPaging:
			if fv.CanAddr() {
				p := fv.Addr().Interface().(*community.Paging)
				b.paging = append(b.paging, Pagination(p))
			} else {
				p := fv.Interface().(community.Paging)
				b.paging = append(b.paging, Pagination(&p))
			}
			continue
		case typCommunityOrderPtr:
			if fv.IsNil() {
				continue
			}
			o := fv.Interface().(*community.Order)
			bind := orderBindMapForField(ft, bindOrders)
			b.order = append(b.order, Orders(*o, bind))
			continue
		case typCommunityOrder:
			o := fv.Interface().(community.Order)
			bind := orderBindMapForField(ft, bindOrders)
			b.order = append(b.order, Orders(o, bind))
			continue
		}

		tag := ft.Tag.Get("filter")
		if tag != "" && !isZeroValue(fv) {
			parts := strings.SplitN(tag, ",", 2)
			if len(parts) < 2 {
				continue
			}
			colExpr, opExpr := parts[0], parts[1]
			b.where = append(b.where, filterQueryOption(colExpr, opExpr, fv.Interface()))
			continue
		}

		if ft.Type.Kind() == reflect.Struct {
			nested := fv.Interface()
			sub := collectQueryOptionBuckets(nested, bindOrders...)
			b.appendAll(sub)
		}
	}

	return b
}

// BuildQueryOptions 从带 filter tag 的结构体自动构建 QueryOption 切片。
//
// criteria 为 nil 时返回 nil。
//
// filter tag 格式: `filter:"column,operator"`，operator 只能是单个 token（不可含 & 与 |）。
//
// 支持的操作符:
//   - 比较: eq, ne, gt, lt, ge, le (或直接用 =, <>, >, <, >=, <=)
//   - 模糊: like, ilike
//   - 集合: in, not_in
//   - 多列 OR: 列名用 | 分隔，如 name|description,like
//
// 零值与指针:
//   - 非指针字段为零值时不生成条件
//   - 指针仅 nil 跳过；*int(0) 等仍会生成条件
//
// 特殊字段:
//   - community.Paging 或 *community.Paging：非 nil 时使用 Pagination
//   - community.Order 或 *community.Order：使用 Orders；*Order 为 nil 则忽略
//   - 嵌套结构体递归处理
//
// 选项顺序（与字段在结构体中的书写顺序无关）:
//  1. Where 2. Pagination 3. Orders
func BuildQueryOptions(criteria interface{}, bindOrders ...map[string]string) []QueryOption {
	b := collectQueryOptionBuckets(criteria, bindOrders...)
	n := len(b.where) + len(b.paging) + len(b.order)
	if n == 0 {
		return nil
	}
	out := make([]QueryOption, 0, n)
	out = append(out, b.where...)
	out = append(out, b.paging...)
	out = append(out, b.order...)
	return out
}
