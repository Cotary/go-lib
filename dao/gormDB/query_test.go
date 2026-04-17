package gormDB

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ===== 零值 / nil 工具 =====

type customZero struct{ x int }

func (c customZero) IsZero() bool { return c.x == 0 }

func TestIsZero_Basic(t *testing.T) {
	assert.True(t, IsZero(nil))
	assert.True(t, IsZero(""))
	assert.True(t, IsZero(int64(0)))
	assert.False(t, IsZero("x"))
	assert.False(t, IsZero(int64(1)))
}

func TestIsZero_PointerAndCustom(t *testing.T) {
	var p *int
	assert.True(t, IsZero(p))

	v := 0
	assert.True(t, IsZero(&v))

	v = 1
	assert.False(t, IsZero(&v))

	assert.True(t, IsZero(customZero{}))
	assert.False(t, IsZero(customZero{x: 1}))
}

func TestDeref(t *testing.T) {
	v := int64(7)
	assert.Equal(t, int64(7), Deref(&v))
	assert.Equal(t, "abc", Deref("abc"))
	var p *int64
	assert.Nil(t, Deref(p))
}

func TestIsNilPtr(t *testing.T) {
	var p *int64
	assert.True(t, IsNilPtr(p))
	v := int64(1)
	assert.False(t, IsNilPtr(&v))
	assert.False(t, IsNilPtr(int64(0)))
}

// ===== Where 系列 =====

func TestWhereIf_SkipsZeroAndNil(t *testing.T) {
	db := openDryRunDB(t)
	var p *int64
	_, sql := buildSQL(db,
		WhereIf("name", OpEq, ""),
		WhereIf("age", OpEq, int64(0)),
		WhereIf("parent_id", OpEq, p),
	)
	assert.NotContains(t, strings.ToLower(sql), "where")
}

func TestWhereIf_AppliesNonZero(t *testing.T) {
	db := openDryRunDB(t)
	v := int64(5)
	_, sql := buildSQL(db,
		WhereIf("name", OpEq, "alice"),
		WhereIf("age", OpGte, int64(18)),
		WhereIf("parent_id", OpEq, &v),
	)
	assert.Contains(t, sql, `name = "alice"`)
	assert.Contains(t, sql, `age >= 18`)
	assert.Contains(t, sql, `parent_id = 5`)
}

func TestWhereAlways_NilPtrFoldsToZero(t *testing.T) {
	db := openDryRunDB(t)
	var p *int64
	_, sql := buildSQL(db,
		WhereAlways("status", OpEq, p),
		WhereAlways("name", OpEq, ""),
	)
	assert.Contains(t, sql, "status = 0")
	assert.Contains(t, sql, `name = ""`)
}

func TestWhereNullable(t *testing.T) {
	db := openDryRunDB(t)
	var p *int64
	_, sqlNull := buildSQL(db, WhereNullable("parent_id", OpEq, p))
	assert.Contains(t, strings.ToUpper(sqlNull), "PARENT_ID IS NULL")

	_, sqlNotNull := buildSQL(db, WhereNullable("parent_id", OpNeq, p))
	assert.Contains(t, strings.ToUpper(sqlNotNull), "PARENT_ID IS NOT NULL")

	v := int64(5)
	_, sqlVal := buildSQL(db, WhereNullable("parent_id", OpEq, &v))
	assert.Contains(t, sqlVal, "parent_id = 5")
}

func TestIsNullAndIsNotNull(t *testing.T) {
	db := openDryRunDB(t)
	_, sql1 := buildSQL(db, IsNull("deleted_at"))
	assert.Contains(t, strings.ToUpper(sql1), "DELETED_AT IS NULL")

	_, sql2 := buildSQL(db, IsNotNull("deleted_at"))
	assert.Contains(t, strings.ToUpper(sql2), "DELETED_AT IS NOT NULL")
}

func TestWhereOps(t *testing.T) {
	db := openDryRunDB(t)
	_, sql := buildSQL(db,
		WhereIf("a", OpNeq, int64(1)),
		WhereIf("b", OpLt, int64(2)),
		WhereIf("c", OpLte, int64(3)),
		WhereIf("d", OpGt, int64(4)),
		WhereIf("e", OpGte, int64(5)),
		WhereIf("f", OpIn, []int64{6, 7}),
		WhereIf("g", OpLike, "kw"),
	)
	assert.Contains(t, sql, "a <> 1")
	assert.Contains(t, sql, "b < 2")
	assert.Contains(t, sql, "c <= 3")
	assert.Contains(t, sql, "d > 4")
	assert.Contains(t, sql, "e >= 5")
	assert.Contains(t, sql, "f IN (6,7)")
	assert.Contains(t, sql, `g LIKE "%kw%"`)
}

// ===== 分页 =====

func TestPaginate_BasicLimitOffset(t *testing.T) {
	db := openDryRunDB(t)
	p := &Paging{Page: 2, PageSize: 10}
	_, sql := buildSQL(db, Paginate(p))
	lower := strings.ToLower(sql)
	assert.Contains(t, lower, "limit 10")
	assert.Contains(t, lower, "offset 10")
}

func TestPaginate_AllSkipsLimit(t *testing.T) {
	db := openDryRunDB(t)
	p := &Paging{All: true, Page: 1, PageSize: 10}
	_, sql := buildSQL(db, Paginate(p))
	assert.NotContains(t, strings.ToLower(sql), "limit")
}

func TestPaginate_DefaultsAndNilSafe(t *testing.T) {
	db := openDryRunDB(t)
	// nil 入参是 no-op
	_, sqlNil := buildSQL(db, Paginate(nil))
	assert.NotContains(t, strings.ToLower(sqlNil), "limit")

	// 默认 page=1 / size=20；offset=0 时 GORM 不输出 OFFSET 子句，仅断言 LIMIT 默认值。
	p := &Paging{}
	_, sql := buildSQL(db, Paginate(p))
	lower := strings.ToLower(sql)
	assert.Contains(t, lower, "limit 20")
}

// ===== 排序 =====

func TestOrderWhitelist_Allowed(t *testing.T) {
	db := openDryRunDB(t)
	o := Order{OrderField: "created_at", OrderType: "DESC"}
	allowed := map[string]string{"created_at": "t.created_at"}
	_, sql := buildSQL(db, OrderWhitelist(o, allowed))
	assert.Contains(t, sql, "t.created_at DESC")
}

func TestOrderWhitelist_NotAllowedSkipped(t *testing.T) {
	db := openDryRunDB(t)
	o := Order{OrderField: "evil_col", OrderType: "DESC"}
	allowed := map[string]string{"created_at": "t.created_at"}
	_, sql := buildSQL(db, OrderWhitelist(o, allowed))
	assert.NotContains(t, strings.ToLower(sql), "order by")
}

func TestOrderWhitelist_DefaultAsc(t *testing.T) {
	db := openDryRunDB(t)
	o := Order{OrderField: "id", OrderType: "weird"}
	allowed := map[string]string{"id": "t.id"}
	_, sql := buildSQL(db, OrderWhitelist(o, allowed))
	assert.Contains(t, sql, "t.id ASC")
}

func TestOrderBy_Passthrough(t *testing.T) {
	db := openDryRunDB(t)
	_, sql := buildSQL(db, OrderBy("id DESC"))
	assert.Contains(t, sql, "id DESC")

	_, sqlEmpty := buildSQL(db, OrderBy("  "))
	assert.NotContains(t, strings.ToLower(sqlEmpty), "order by")
}

// ===== 透传 Scope =====

func TestPassthroughScopes(t *testing.T) {
	db := openDryRunDB(t)
	_, sql := buildSQL(db,
		WhereRaw("status = ?", 1),
		LimitN(5),
		OffsetN(10),
	)
	lower := strings.ToLower(sql)
	assert.Contains(t, sql, "status = 1")
	assert.Contains(t, lower, "limit 5")
	assert.Contains(t, lower, "offset 10")
}

// SQLite 不支持 SELECT ... FOR UPDATE，DryRun 模式下方言会跳过该子句。
// 这里只验证 Locking clause 已注入到 Statement，避免误报。
func TestForUpdate_ClauseInjected(t *testing.T) {
	db := openDryRunDB(t)
	tx := db.Scopes(ForUpdate()).Find(&[]TestRow{})
	_, ok := tx.Statement.Clauses["FOR"]
	assert.True(t, ok, "expected Locking clause injected by ForUpdate()")
}
