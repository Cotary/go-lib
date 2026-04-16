package gormDB

// ---------------------------------------------------------------------------
// FilterBuilder — 链式查询条件构造器
//
// 用法：
//
//	opts := gormDB.Filter().
//	    Eq("status", req.Status).             // 零值自动跳过
//	    MustEq("mchid", req.Mchid).           // 不检查零值，仅 nil 跳过
//	    MustGte("score", req.MinScore).        // 同上，适用于所有操作符
//	    ILike("name", req.Name).
//	    Gte("created_at", req.StartTime).
//	    Lte("created_at", req.EndTime).
//	    Option(gormDB.Pagination(paging)).     // nil 安全
//	    Option(gormDB.Order("id DESC")).
//	    Build()
//
// Xxx 系列（Eq/Neq/Gt 等）：nil 和零值参数自动跳过。
// MustXxx 系列（MustEq/MustGt 等）：仅 nil 跳过，零值也生成条件。
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

// ---------------------------------------------------------------------------
// MustXxx 系列 — 强制添加条件，不做零值检查，仅 nil 跳过。
// 用于非指针字段的零值也是有效业务值的场景（如查询 status=0 的记录）。
// ---------------------------------------------------------------------------

// MustEq 强制等值（=），不做零值检查，仅 nil 跳过。
func (b *FilterBuilder) MustEq(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, mustFilterQueryOption(col, "=", val))
	return b
}

// MustNeq 强制不等（<>），不做零值检查，仅 nil 跳过。
func (b *FilterBuilder) MustNeq(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, mustFilterQueryOption(col, "<>", val))
	return b
}

// MustGt 强制大于（>），不做零值检查，仅 nil 跳过。
func (b *FilterBuilder) MustGt(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, mustFilterQueryOption(col, ">", val))
	return b
}

// MustLt 强制小于（<），不做零值检查，仅 nil 跳过。
func (b *FilterBuilder) MustLt(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, mustFilterQueryOption(col, "<", val))
	return b
}

// MustGte 强制大于等于（>=），不做零值检查，仅 nil 跳过。
func (b *FilterBuilder) MustGte(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, mustFilterQueryOption(col, ">=", val))
	return b
}

// MustLte 强制小于等于（<=），不做零值检查，仅 nil 跳过。
func (b *FilterBuilder) MustLte(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, mustFilterQueryOption(col, "<=", val))
	return b
}

// MustLike 强制模糊查询（%val%），不做零值检查，仅 nil 跳过。
func (b *FilterBuilder) MustLike(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, mustFilterQueryOption(col, "like", val))
	return b
}

// MustILike 强制大小写不敏感模糊查询（PostgreSQL），不做零值检查，仅 nil 跳过。
func (b *FilterBuilder) MustILike(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, mustFilterQueryOption(col, "ilike", val))
	return b
}

// MustIn 强制集合包含查询，不做零值检查，仅 nil 跳过。
func (b *FilterBuilder) MustIn(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, mustFilterQueryOption(col, "in", val))
	return b
}

// MustNotIn 强制集合排除查询，不做零值检查，仅 nil 跳过。
func (b *FilterBuilder) MustNotIn(col string, val interface{}) *FilterBuilder {
	b.opts = append(b.opts, mustFilterQueryOption(col, "not_in", val))
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
