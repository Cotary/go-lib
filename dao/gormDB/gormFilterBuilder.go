package gormDB

import (
	"fmt"
	"reflect"
)

// ---------------------------------------------------------------------------
// FilterBuilder — 链式查询条件构造器
//
// 用法：
//
//	opts := gormDB.Filter().
//	    Eq("status", req.Status).          // 零值自动跳过
//	    Must("mchid", req.Mchid).          // 不检查零值，nil 跳过
//	    ILike("name", req.Name).
//	    Gte("created_at", req.StartTime).
//	    Lte("created_at", req.EndTime).
//	    Option(community.Pagination(paging)).  // nil 安全
//	    Option(gormDB.Order("id DESC")).
//	    Build()
//
// 所有过滤方法（Eq/Neq/Gt 等）对 nil 和零值参数自动跳过。
// Option/Options 对 nil QueryOption 自动跳过。
// ---------------------------------------------------------------------------

type FilterBuilder struct {
	opts []QueryOption
}

// Filter 创建一个新的 FilterBuilder。
func Filter() *FilterBuilder {
	return &FilterBuilder{}
}

// Eq 等值（=），val 为零值时自动跳过。
func (b *FilterBuilder) Eq(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "=", val))
	return b
}

// Neq 不等（<>），val 为零值时自动跳过。
func (b *FilterBuilder) Neq(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "<>", val))
	return b
}

// Gt 大于（>），val 为零值时自动跳过。
func (b *FilterBuilder) Gt(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, ">", val))
	return b
}

// Lt 小于（<），val 为零值时自动跳过。
func (b *FilterBuilder) Lt(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "<", val))
	return b
}

// Gte 大于等于（>=），val 为零值时自动跳过。
func (b *FilterBuilder) Gte(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, ">=", val))
	return b
}

// Lte 小于等于（<=），val 为零值时自动跳过。
func (b *FilterBuilder) Lte(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "<=", val))
	return b
}

// Like 模糊查询（%val%），val 为零值时自动跳过。
func (b *FilterBuilder) Like(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "like", val))
	return b
}

// ILike 大小写不敏感模糊查询（PostgreSQL），val 为零值时自动跳过。
func (b *FilterBuilder) ILike(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "ilike", val))
	return b
}

// In 集合包含查询，val 为空切片时自动跳过。
func (b *FilterBuilder) In(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "in", val))
	return b
}

// NotIn 集合排除查询，val 为空切片时自动跳过。
func (b *FilterBuilder) NotIn(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "not_in", val))
	return b
}

// Between 区间查询（col BETWEEN start AND end）。
// start 或 end 为 nil / 零值时跳过。
func (b *FilterBuilder) Between(col string, start, end interface{}) *FilterBuilder {
	if isZeroInterface(start) || isZeroInterface(end) {
		return b
	}
	sv, ev := reflect.ValueOf(start), reflect.ValueOf(end)
	if !sv.IsValid() || isZeroValue(sv) || !ev.IsValid() || isZeroValue(ev) {
		return b
	}
	b.opts = append(b.opts, Where(fmt.Sprintf("%s BETWEEN ? AND ?", col), start, end))
	return b
}

// Must 强制添加等值条件（不做零值检查），用于 id 等必填字段。
// val 为 nil 时跳过。
func (b *FilterBuilder) Must(col string, val interface{}) *FilterBuilder {
	if val == nil {
		return b
	}
	b.opts = append(b.opts, Where(col+" = ?", val))
	return b
}

// Option 注入单个 QueryOption（如 Pagination、Order 或自定义复杂条件）。
// opt 为 nil 时跳过。
func (b *FilterBuilder) Option(opt QueryOption) *FilterBuilder {
	if opt != nil {
		b.opts = append(b.opts, opt)
	}
	return b
}

// Options 注入多个 QueryOption，自动过滤 nil 项。
func (b *FilterBuilder) Options(opts ...QueryOption) *FilterBuilder {
	for _, opt := range opts {
		if opt != nil {
			b.opts = append(b.opts, opt)
		}
	}
	return b
}

// Build 返回构建好的 []QueryOption，可直接传入 model 的 List / Get 方法。
func (b *FilterBuilder) Build() []QueryOption {
	return b.opts
}
