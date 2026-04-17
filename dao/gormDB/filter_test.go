package gormDB

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ===== 基础 tag 解析 =====

func TestApplyFilter_DefaultIf(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		Name string `filter:"name,="`
		Age  int64  `filter:"age,gte"`
	}
	f := Req{Name: "alice", Age: 18}
	_, sql := buildSQL(db, ApplyFilter(f))
	assert.Contains(t, sql, `name = "alice"`)
	assert.Contains(t, sql, `age >= 18`)
}

func TestApplyFilter_ZeroSkipped(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		Name string `filter:"name,="`
		Age  int64  `filter:"age,gte"`
	}
	f := Req{}
	_, sql := buildSQL(db, ApplyFilter(f))
	assert.NotContains(t, strings.ToLower(sql), "where")
}

func TestApplyFilter_AlwaysMode(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		TenantID int64 `filter:"tenant_id,=,always"`
		Status   int64 `filter:"status,=,always"`
	}
	f := Req{TenantID: 0, Status: 0}
	_, sql := buildSQL(db, ApplyFilter(f))
	assert.Contains(t, sql, `tenant_id = 0`)
	assert.Contains(t, sql, `status = 0`)
}

func TestApplyFilter_AlwaysMode_NilPtrToZero(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		Cnt *int64 `filter:"cnt,=,always"`
	}
	f := Req{}
	_, sql := buildSQL(db, ApplyFilter(f))
	assert.Contains(t, sql, `cnt = 0`)
}

func TestApplyFilter_NullableMode(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		ParentID *int64 `filter:"parent_id,=,nullable"`
	}
	f := Req{}
	_, sql := buildSQL(db, ApplyFilter(f))
	assert.Contains(t, strings.ToUpper(sql), "PARENT_ID IS NULL")
}

func TestApplyFilter_NullableMode_NonNil(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		ParentID *int64 `filter:"parent_id,=,nullable"`
	}
	v := int64(5)
	f := Req{ParentID: &v}
	_, sql := buildSQL(db, ApplyFilter(f))
	assert.Contains(t, sql, `parent_id = 5`)
}

// ===== 多列 OR =====

func TestApplyFilter_MultiColOrLike(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		Keyword string `filter:"name|description,like"`
	}
	f := Req{Keyword: "kw"}
	_, sql := buildSQL(db, ApplyFilter(f))
	assert.Contains(t, sql, `name LIKE "%kw%"`)
	assert.Contains(t, sql, `description LIKE "%kw%"`)
	assert.Contains(t, sql, "OR")
}

// ===== Paging / Order 字段 =====

func TestApplyFilter_Paging(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		Paging Paging
		Name   string `filter:"name,="`
	}
	f := Req{Paging: Paging{Page: 2, PageSize: 5}, Name: "x"}
	_, sql := buildSQL(db, ApplyFilter(f))
	lower := strings.ToLower(sql)
	assert.Contains(t, lower, "limit 5")
	assert.Contains(t, lower, "offset 5")
}

func TestApplyFilter_OrderWithAllowed(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		Order Order
	}
	f := Req{Order: Order{OrderField: "created_at", OrderType: "desc"}}
	allowed := map[string]string{"created_at": "t.created_at"}
	_, sql := buildSQL(db, ApplyFilter(f, allowed))
	assert.Contains(t, sql, "t.created_at DESC")
}

// SQL 子句顺序应当是 WHERE → ORDER BY → LIMIT，借此验证 ApplyFilter 内部顺序处理。
func TestApplyFilter_SqlClauseOrder(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		Paging Paging
		Order  Order
		Name   string `filter:"name,like"`
	}
	allowed := map[string]string{"created_at": "t.created_at"}
	f := Req{
		Paging: Paging{Page: 1, PageSize: 10},
		Order:  Order{OrderField: "created_at", OrderType: "desc"},
		Name:   "kw",
	}
	_, sql := buildSQL(db, ApplyFilter(f, allowed))
	lower := strings.ToLower(sql)

	iWhere := strings.Index(lower, "where")
	iOrder := strings.Index(lower, "order by")
	iLimit := strings.Index(lower, "limit")
	assert.GreaterOrEqual(t, iWhere, 0)
	assert.Greater(t, iOrder, iWhere)
	assert.Greater(t, iLimit, iOrder)
}

// ===== 指针字段 / 嵌套结构体 =====

func TestApplyFilter_NilPagingPtrSkipped(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		Paging *Paging
		Name   string `filter:"name,="`
	}
	f := Req{Name: "x"}
	_, sql := buildSQL(db, ApplyFilter(f))
	assert.NotContains(t, strings.ToLower(sql), "limit")
	assert.Contains(t, sql, `name = "x"`)
}

type InnerFilter struct {
	Email string `filter:"email,="`
}

func TestApplyFilter_NestedStruct(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		Name string `filter:"name,="`
		InnerFilter
	}
	f := Req{Name: "a", InnerFilter: InnerFilter{Email: "b"}}
	_, sql := buildSQL(db, ApplyFilter(f))
	assert.Contains(t, sql, `name = "a"`)
	assert.Contains(t, sql, `email = "b"`)
}

// ===== 非法 tag =====

func TestApplyFilter_InvalidTagIgnored(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		NoComma   string `filter:"name"`
		UnknownOp string `filter:"name,xxx"`
		EmptyCol  string `filter:",="`
	}
	f := Req{NoComma: "a", UnknownOp: "b", EmptyCol: "c"}
	_, sql := buildSQL(db, ApplyFilter(f))
	assert.NotContains(t, strings.ToLower(sql), "where")
}

func TestApplyFilter_NilCriteriaNoPanic(t *testing.T) {
	db := openDryRunDB(t)
	_, sql := buildSQL(db, ApplyFilter(nil))
	assert.NotContains(t, strings.ToLower(sql), "where")
}

// ===== 别名解析 =====

func TestApplyFilter_TagAliases(t *testing.T) {
	db := openDryRunDB(t)
	type Req struct {
		A int64 `filter:"a,eq"`
		B int64 `filter:"b,neq"`
		C int64 `filter:"c,gt"`
		D int64 `filter:"d,lt"`
		E int64 `filter:"e,gte"`
		F int64 `filter:"f,lte"`
	}
	f := Req{A: 1, B: 2, C: 3, D: 4, E: 5, F: 6}
	_, sql := buildSQL(db, ApplyFilter(f))
	assert.Contains(t, sql, `a = 1`)
	assert.Contains(t, sql, `b <> 2`)
	assert.Contains(t, sql, `c > 3`)
	assert.Contains(t, sql, `d < 4`)
	assert.Contains(t, sql, `e >= 5`)
	assert.Contains(t, sql, `f <= 6`)
}
