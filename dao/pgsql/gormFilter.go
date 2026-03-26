package pgsql

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/Cotary/go-lib/common/community"
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

// parseExpr 将表达式按 | 和 & 拆成 tokens 和 connectors
func parseExpr(s string) (tokens []string, conns []rune) {
	var cur strings.Builder
	for _, r := range s {
		if r == '|' || r == '&' {
			t := strings.TrimSpace(cur.String())
			if t != "" {
				tokens = append(tokens, t)
				conns = append(conns, r)
			}
			cur.Reset()
		} else {
			cur.WriteRune(r)
		}
	}
	if t := strings.TrimSpace(cur.String()); t != "" {
		tokens = append(tokens, t)
	}
	return
}

var opSymMap = map[string]string{
	"eq": "=", "ne": "<>",
	"gt": ">", "lt": "<",
	"ge": ">=", "le": "<=",
}

func buildClauseAndArgs(colExpr, opExpr string, val interface{}) (string, []interface{}) {
	cols, _ := parseExpr(colExpr)
	ops, opConns := parseExpr(opExpr)

	var groupClauses []string
	var args []interface{}

	for _, op := range ops {
		var colClauses []string
		for _, col := range cols {
			switch op {
			case "eq", "ne", "gt", "lt", "ge", "le", "=", "<>", ">", "<", ">=", "<=":
				sym, ok := opSymMap[op]
				if !ok {
					sym = op
				}
				colClauses = append(colClauses, fmt.Sprintf("%s %s ?", col, sym))
				args = append(args, val)

			case "like", "ilike":
				pat := fmt.Sprintf("%%%v%%", val)
				colClauses = append(colClauses, fmt.Sprintf("%s %s ?", col, op))
				args = append(args, pat)

			case "in", "not_in":
				kw := "IN"
				if op == "not_in" {
					kw = "NOT IN"
				}
				colClauses = append(colClauses, fmt.Sprintf("%s %s ?", col, kw))
				args = append(args, val)
			}
		}

		var clause string
		if len(colClauses) > 1 {
			clause = "(" + strings.Join(colClauses, " OR ") + ")"
		} else if len(colClauses) == 1 {
			clause = colClauses[0]
		}
		groupClauses = append(groupClauses, clause)
	}

	if len(groupClauses) == 0 {
		return "", nil
	}

	result := groupClauses[0]
	for i, conn := range opConns {
		next := groupClauses[i+1]
		if conn == '&' {
			result = "(" + result + " AND " + next + ")"
		} else {
			result = "(" + result + " OR " + next + ")"
		}
	}
	return result, args
}

// BuildQueryOptions 从带 filter tag 的结构体自动构建 QueryOption 切片。
//
// filter tag 格式: `filter:"column,operator"`
//
// 支持的操作符:
//   - 比较: eq, ne, gt, lt, ge, le (或直接用 =, <>, >, <, >=, <=)
//   - 模糊: like, ilike
//   - 集合: in, not_in
//   - 区间: bs (begin start), be (begin end) — 配对使用生成 BETWEEN
//   - 组合: 用 & (AND) 和 | (OR) 连接多个操作符，如 ge&le
//   - 多列: 列名也可用 | 分隔，如 name|description,like
//
// 特殊字段:
//   - community.Paging 类型字段自动应用 Pagination
//   - community.Order 类型字段自动应用 Orders（bindOrders 可选）
//   - 嵌套结构体递归处理
//
// 重要：要让 Pagination 的 Total 回写到原始结构体，criteria 必须传指针。
// 传值时 Paging 会降级为仅分页（不计 Total）。
func BuildQueryOptions(criteria interface{}, bindOrders ...map[string]string) []QueryOption {
	var opts []QueryOption

	rv := reflect.ValueOf(criteria)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	rt := rv.Type()

	type pair struct {
		colExpr    string
		start, end interface{}
		hasStart   bool
		hasEnd     bool
	}
	groups := make(map[string]*pair)

	for i := 0; i < rt.NumField(); i++ {
		ft := rt.Field(i)
		fv := rv.Field(i)
		tag := ft.Tag.Get("filter")
		if tag == "" || isZeroValue(fv) {
			continue
		}
		parts := strings.SplitN(tag, ",", 2)
		if len(parts) < 2 {
			continue
		}
		colExpr, opExpr := parts[0], parts[1]

		if opExpr == "bs" || strings.HasPrefix(opExpr, "bs.") {
			key := ""
			if idx := strings.Index(opExpr, "."); idx >= 0 {
				key = opExpr[idx+1:]
			}
			g := groups[key]
			if g == nil {
				g = &pair{colExpr: colExpr}
				groups[key] = g
			}
			g.start = fv.Interface()
			g.hasStart = true
		} else if opExpr == "be" || strings.HasPrefix(opExpr, "be.") {
			key := ""
			if idx := strings.Index(opExpr, "."); idx >= 0 {
				key = opExpr[idx+1:]
			}
			g := groups[key]
			if g == nil {
				g = &pair{colExpr: colExpr}
				groups[key] = g
			}
			g.end = fv.Interface()
			g.hasEnd = true
		}
	}

	for _, g := range groups {
		if !g.hasStart && !g.hasEnd {
			continue
		}
		switch {
		case g.hasStart && g.hasEnd:
			opts = append(opts,
				Where(fmt.Sprintf("%s BETWEEN ? AND ?", g.colExpr), g.start, g.end),
			)
		case g.hasStart:
			opts = append(opts,
				Where(fmt.Sprintf("%s >= ?", g.colExpr), g.start),
			)
		case g.hasEnd:
			opts = append(opts,
				Where(fmt.Sprintf("%s <= ?", g.colExpr), g.end),
			)
		}
	}

	for i := 0; i < rt.NumField(); i++ {
		ft := rt.Field(i)
		fv := rv.Field(i)

		if ft.Type == reflect.TypeOf(community.Paging{}) {
			if fv.CanAddr() {
				p := fv.Addr().Interface().(*community.Paging)
				opts = append(opts, Pagination(p))
			} else {
				p := fv.Interface().(community.Paging)
				opts = append(opts, Paging(&p))
			}
			continue
		}
		if ft.Type == reflect.TypeOf(community.Order{}) {
			o := fv.Interface().(community.Order)
			opts = append(opts, Orders(o, bindOrders...))
			continue
		}

		tag := ft.Tag.Get("filter")
		if tag != "" && !isZeroValue(fv) {
			parts := strings.SplitN(tag, ",", 2)
			if len(parts) < 2 {
				continue
			}
			colExpr, opExpr := parts[0], parts[1]
			if opExpr == "bs" || strings.HasPrefix(opExpr, "bs.") ||
				opExpr == "be" || strings.HasPrefix(opExpr, "be.") {
				continue
			}
			clause, args := buildClauseAndArgs(colExpr, opExpr, fv.Interface())
			if clause != "" {
				opts = append(opts, Where(clause, args...))
			}
			continue
		}

		if ft.Type.Kind() == reflect.Struct {
			nested := fv.Interface()
			opts = append(opts, BuildQueryOptions(nested, bindOrders...)...)
		}
	}

	return opts
}
