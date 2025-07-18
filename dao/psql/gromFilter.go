package psql

import (
	"fmt"
	"reflect"
	"strings"

	// import community 仅剩 Paging 和 Order
	"github.com/Cotary/go-lib/common/community"
)

// isZeroValue 跳过空字符串、零值、nil、空 slice 等
func isZeroValue(v reflect.Value) bool {
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

// buildClauseAndArgs 按列和操作符组合 SQL 及参数
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

		// 组内 OR
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

	// 组间 &/| 串联
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

// BuildQueryOptions 支持 bs.xxx / be.xxx 自动配对，并生成 BETWEEN 或 >=/<= 子句
func BuildQueryOptions(criteria interface{}, bindOrders ...map[string]string) []QueryOption {
	var opts []QueryOption

	rv := reflect.ValueOf(criteria)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	rt := rv.Type()

	// 1) 预扫描 bs/be 配对字段
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
		colExpr, opExpr := parts[0], parts[1]

		// bs / be / bs.id / be.id
		if strings.HasPrefix(opExpr, "bs") {
			// bs or bs.<id>
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
		} else if strings.HasPrefix(opExpr, "be") {
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

	// 2) 生成所有 between / >= / <= 子句
	for _, g := range groups {
		// 只在至少一个端点非零时才处理
		if !g.hasStart && !g.hasEnd {
			continue
		}
		switch {
		case g.hasStart && g.hasEnd:
			opts = append(opts,
				WithWhere(fmt.Sprintf("%s BETWEEN ? AND ?", g.colExpr), g.start, g.end),
			)
		case g.hasStart:
			opts = append(opts,
				WithWhere(fmt.Sprintf("%s >= ?", g.colExpr), g.start),
			)
		case g.hasEnd:
			opts = append(opts,
				WithWhere(fmt.Sprintf("%s <= ?", g.colExpr), g.end),
			)
		}
	}

	// 第二遍：常规过滤、分页、排序、嵌套 struct
	for i := 0; i < rt.NumField(); i++ {
		ft := rt.Field(i)
		fv := rv.Field(i)

		// 分页
		if ft.Type == reflect.TypeOf(community.Paging{}) {
			p := fv.Interface().(community.Paging)
			opts = append(opts, WithPagination(&p))
			continue
		}
		// 排序
		if ft.Type == reflect.TypeOf(community.Order{}) {
			o := fv.Interface().(community.Order)
			opts = append(opts, WithOrders(o, bindOrders...))
			continue
		}

		tag := ft.Tag.Get("filter")
		if tag != "" && !isZeroValue(fv) {
			parts := strings.SplitN(tag, ",", 2)
			colExpr, opExpr := parts[0], parts[1]
			if strings.HasPrefix(opExpr, "bs.") || strings.HasPrefix(opExpr, "be.") {
				// 已在配对阶段处理，跳过
				continue
			}
			clause, args := buildClauseAndArgs(colExpr, opExpr, fv.Interface())
			if clause != "" {
				opts = append(opts, WithWhere(clause, args...))
			}
			continue
		}

		// 嵌套 struct
		if ft.Type.Kind() == reflect.Struct {
			nested := fv.Interface()
			opts = append(opts, BuildQueryOptions(nested, bindOrders...)...)
		}
	}

	return opts
}
