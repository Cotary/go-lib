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
//	    Eq(req.Status, "status").        // 零值自动跳过
//	    Must("mchid", req.Mchid).        // 不检查零值
//	    ILike(req.Name, "name").
//	    Gte(req.StartTime, "created_at").
//	    Lte(req.EndTime, "created_at").
//	    Option(community.Pagination(paging)).  // nil 安全
//	    Option(gormDB.Order("id DESC")).
//	    Build()
//
// 所有过滤方法（Eq/Ne/Gt 等）对 nil 和零值参数自动跳过。
// Option/Options 对 nil QueryOption 自动跳过。
// ---------------------------------------------------------------------------

type FilterBuilder struct {
	opts []QueryOption
}

// Filter 创建一个新的 FilterBuilder。
func Filter() *FilterBuilder {
	return &FilterBuilder{}
}

func (b *FilterBuilder) Eq(val interface{}, col string) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "eq", val))
	return b
}

func (b *FilterBuilder) Ne(val interface{}, col string) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "ne", val))
	return b
}

func (b *FilterBuilder) Gt(val interface{}, col string) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "gt", val))
	return b
}

func (b *FilterBuilder) Lt(val interface{}, col string) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "lt", val))
	return b
}

func (b *FilterBuilder) Gte(val interface{}, col string) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "ge", val))
	return b
}

func (b *FilterBuilder) Lte(val interface{}, col string) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "le", val))
	return b
}

func (b *FilterBuilder) Like(val interface{}, col string) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "like", val))
	return b
}

func (b *FilterBuilder) ILike(val interface{}, col string) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "ilike", val))
	return b
}

func (b *FilterBuilder) In(val interface{}, col string) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "in", val))
	return b
}

func (b *FilterBuilder) NotIn(val interface{}, col string) *FilterBuilder {
	b.opts = append(b.opts, filterQueryOption(col, "not_in", val))
	return b
}

// Between 区间查询，仅当 start 和 end 都非 nil 且非零值时才生成条件。
func (b *FilterBuilder) Between(start, end interface{}, col string) *FilterBuilder {
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
