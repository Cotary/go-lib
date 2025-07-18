package psql

import (
	"fmt"
	"github.com/Cotary/go-lib/common/community"
	"gorm.io/gorm"
	"strings"
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

// WithPagination 先计算 TotalCount，再根据 Paging 做分页
func WithPagination(p *community.Paging) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		// 1. 设置默认分页参数
		if p.PageSize < 1 {
			p.PageSize = pageSize
		}
		if p.Page < 1 {
			p.Page = page
		}

		// 2. 统计总数（清除可能已有的 Limit/Offset）
		db = db.
			Session(&gorm.Session{}).
			Limit(-1).
			Offset(-1).
			Count(&p.TotalCount)
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

func WithPaging(p *community.Paging) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
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
func WithTotal(count *int64) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		// 设置为 -1 避免分页影响总数
		return db.
			Limit(-1).
			Offset(-1).
			Count(count)
	}
}
func WithOrders(o community.Order, bindMaps ...map[string]string) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		var clauses []string
		bind := map[string]string{}
		if len(bindMaps) > 0 {
			bind = bindMaps[0]
		}

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
				// 自定义模板里有 {order_type} 占位
				clauses = append(clauses, strings.ReplaceAll(tpl, orderType, ord))
			} else {
				// 默认拼 field + " " + ord
				clauses = append(clauses, fmt.Sprintf("%s %s", f, ord))
			}
		}

		if len(clauses) == 0 {
			return db
		}
		return db.Order(strings.Join(clauses, ", "))
	}
}

func WithTable(tableName string) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Table(tableName)
	}
}

func WithWhere(query interface{}, args ...interface{}) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(query, args...)
	}
}

func WithOrder(order string) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Order(order)
	}
}
func WithLimit(n int) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Limit(n)
	}
}

func WithOffset(n int) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Offset(n)
	}
}

func WithID(id int64) QueryOption {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("id = ?", id)
	}
}
