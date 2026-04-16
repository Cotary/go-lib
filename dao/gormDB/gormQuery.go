package gormDB

import (
	"strings"

	"github.com/Cotary/go-lib/common/community"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	page      = 1
	pageSize  = 20
	asc       = "asc"
	desc      = "desc"
	orderType = "{order_type}"
)

// QueryOption 是对 *gorm.DB 的装饰器
type QueryOption func(*gorm.DB) *gorm.DB

// Pagination 先计算 TotalCount，再根据 Paging 做分页。
// 注意：Pagination 必须放在所有 Where 条件之后使用，否则 COUNT 结果不包含后续条件。
func Pagination(p *community.Paging) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		if p == nil {
			return db
		}
		// 1. 设置默认分页参数
		if p.PageSize < 1 {
			p.PageSize = pageSize
		}
		if p.Page < 1 {
			p.Page = page
		}

		// 2. 克隆一个临时 DB 用来做 Count，不影响主 db 链
		var total int64
		db.Session(&gorm.Session{}).
			Limit(-1).
			Offset(-1).Count(&total)
		p.Total = total

		// 3. 如果请求全部，则不再加分页
		if p.All {
			return db
		}

		// 4. 否则应用分页
		return db.
			Limit(p.PageSize).
			Offset((p.Page - 1) * p.PageSize)
	}
}

func Paging(p *community.Paging) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		if p == nil {
			return db
		}
		if p.PageSize < 1 {
			p.PageSize = 20
		}
		if p.Page < 1 {
			p.Page = 1
		}
		if p.All {
			return db
		}
		return db.
			Limit(p.PageSize).
			Offset((p.Page - 1) * p.PageSize)
	}
}
func Total(count *int64) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		// 设置为 -1 避免分页影响总数
		return db.
			Limit(-1).
			Offset(-1).
			Count(count)
	}
}

// Orders 根据 Order 结构体生成排序子句（白名单模式）。
// bindMaps 的 key 为前端传入的排序字段名，value 为实际 SQL 模板（可含 {order_type} 占位符）。
// 只有 bindMaps 中声明的字段才允许排序，未声明的字段直接忽略。
// 当 bindMaps 为空或未提供时，不做任何排序。
// 若通过 BuildQueryOptions 构建且 Order 字段带 `filter:"...,order"`，白名单可在 tag 中声明，见 gormFilterStruct.go 注释。
func Orders(o community.Order, bindMaps ...map[string]string) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		if len(bindMaps) == 0 || bindMaps[0] == nil || len(bindMaps[0]) == 0 {
			return db
		}
		bind := bindMaps[0]

		var clauses []string
		fields := strings.Split(o.OrderField, ",")
		types := strings.Split(o.OrderType, ",")

		for i, raw := range fields {
			f := strings.TrimSpace(raw)
			if f == "" {
				continue
			}
			ord := asc
			if i < len(types) && strings.ToLower(strings.TrimSpace(types[i])) == desc {
				ord = desc
			}

			if tpl, ok := bind[f]; ok {
				clauses = append(clauses, strings.ReplaceAll(tpl, orderType, ord))
			}
		}

		if len(clauses) == 0 {
			return db
		}
		return db.Order(strings.Join(clauses, ", "))
	}
}

func Table(tableName string) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Table(tableName)
	}
}

func Where(query interface{}, args ...interface{}) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(query, args...)
	}
}

func Order(order string) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Order(order)
	}
}

func Limit(n int) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Limit(n)
	}
}

func Offset(n int) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Offset(n)
	}
}

func ID(id int64) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("id = ?", id)
	}
}

func ForUpdate() QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
}

func ForShare() QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Clauses(clause.Locking{Strength: "SHARE"})
	}
}

func Unscoped() QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Unscoped()
	}
}

func Preload(column string, conditions ...interface{}) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Preload(column, conditions...)
	}
}
