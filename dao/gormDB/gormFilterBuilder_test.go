package gormDB

import (
	"strings"
	"testing"

	"github.com/Cotary/go-lib/common/community"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// FilterBuilder（链式构造器模式）测试
// ---------------------------------------------------------------------------

func TestFilterBuilder_BasicEq(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Eq(int64(42), "created_at").
		Eq("alice", "name").
		Build()
	_, realSQL := buildSQL(db, opts)
	t.Logf("Real SQL: %s", realSQL)
	assert.Contains(t, realSQL, "created_at = 42")
	assert.Contains(t, realSQL, `name = "alice"`)
}

func TestFilterBuilder_ZeroValuesSkipped(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Eq(int64(0), "status").
		Eq("", "name").
		Eq(int64(5), "id").
		Build()
	_, realSQL := buildSQL(db, opts)
	t.Logf("Real SQL: %s", realSQL)
	assert.NotContains(t, realSQL, "status")
	assert.NotContains(t, realSQL, "name")
	assert.Contains(t, realSQL, "id = 5")
}

func TestFilterBuilder_AllOperators(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Eq(int64(1), "a").
		Ne(int64(2), "b").
		Gt(int64(3), "c").
		Lt(int64(4), "d").
		Gte(int64(5), "e").
		Lte(int64(6), "f").
		Like("kw", "g").
		ILike("kw2", "h").
		In([]int64{7, 8}, "i").
		NotIn([]int64{9}, "j").
		Build()
	_, realSQL := buildSQL(db, opts)
	t.Logf("Real SQL: %s", realSQL)
	assert.Contains(t, realSQL, "a = 1")
	assert.Contains(t, realSQL, "b <> 2")
	assert.Contains(t, realSQL, "c > 3")
	assert.Contains(t, realSQL, "d < 4")
	assert.Contains(t, realSQL, "e >= 5")
	assert.Contains(t, realSQL, "f <= 6")
	assert.Contains(t, realSQL, "g like")
	assert.Contains(t, realSQL, "h ilike")
	assert.Contains(t, realSQL, "i IN")
	assert.Contains(t, realSQL, "j NOT IN")
}

func TestFilterBuilder_Between(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Between(int64(100), int64(200), "created_at").
		Build()
	_, realSQL := buildSQL(db, opts)
	t.Logf("Real SQL: %s", realSQL)
	assert.Contains(t, realSQL, "created_at BETWEEN")
	assert.Contains(t, realSQL, "100")
	assert.Contains(t, realSQL, "200")
}

func TestFilterBuilder_BetweenSkipsPartialZero(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Between(int64(0), int64(200), "created_at").
		Eq(int64(1), "id").
		Build()
	_, realSQL := buildSQL(db, opts)
	t.Logf("Real SQL: %s", realSQL)
	assert.NotContains(t, realSQL, "BETWEEN")
	assert.Contains(t, realSQL, "id = 1")
}

func TestFilterBuilder_BetweenNilSkipped(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Between(nil, int64(200), "created_at").
		Eq(int64(1), "id").
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.NotContains(t, realSQL, "BETWEEN")
	assert.Contains(t, realSQL, "id = 1")
}

func TestFilterBuilder_Must(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Must("mchid", int64(0)).
		Eq(int64(0), "status").
		Build()
	_, realSQL := buildSQL(db, opts)
	t.Logf("Real SQL: %s", realSQL)
	assert.Contains(t, realSQL, "mchid = 0")
	assert.NotContains(t, realSQL, "status")
}

func TestFilterBuilder_MustNilSkipped(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Must("mchid", nil).
		Eq(int64(1), "id").
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.NotContains(t, realSQL, "mchid")
	assert.Contains(t, realSQL, "id = 1")
}

func TestFilterBuilder_OptionAndOptions(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Eq(int64(1), "status").
		Option(Order("created_at DESC")).
		Options(Where("name IS NOT NULL"), Limit(10)).
		Build()
	placeholder, _ := buildSQL(db, opts)
	t.Logf("Placeholder SQL: %s", placeholder)
	lower := strings.ToLower(placeholder)
	assert.Contains(t, lower, "status = ?")
	assert.Contains(t, lower, "order by")
	assert.Contains(t, lower, "name is not null")
	assert.Contains(t, lower, "limit")
}

func TestFilterBuilder_OptionNilSafe(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Eq(int64(1), "id").
		Option(nil).
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.Contains(t, realSQL, "id = 1")
}

func TestFilterBuilder_OptionsNilSafe(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Eq(int64(1), "id").
		Options(nil, Where("status = ?", 2), nil).
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.Contains(t, realSQL, "id = 1")
	assert.Contains(t, realSQL, "status = 2")
}

func TestFilterBuilder_PaginationNilSafe(t *testing.T) {
	db := openDryRunDB(t)
	var p *community.Paging
	opts := Filter().
		Eq(int64(1), "id").
		Option(Pagination(p)).
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.Contains(t, realSQL, "id = 1")
	assert.NotContains(t, strings.ToLower(realSQL), "limit")
}

func TestFilterBuilder_OrderEmptySafe(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Eq(int64(1), "id").
		Option(Order("")).
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.Contains(t, realSQL, "id = 1")
	assert.NotContains(t, strings.ToLower(realSQL), "order by")
}

func TestFilterBuilder_NilValInEq(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Eq(nil, "status").
		Eq(int64(1), "id").
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.NotContains(t, realSQL, "status")
	assert.Contains(t, realSQL, "id = 1")
}

func TestFilterBuilder_MatchesStandaloneFunctions(t *testing.T) {
	db := openDryRunDB(t)
	_, fromBuilder := buildSQL(db, Filter().Eq(int64(42), "created_at").Build())
	_, fromFn := buildSQL(db, []QueryOption{FilterEq(int64(42), "created_at")})
	assert.Equal(t, fromBuilder, fromFn)
}

func TestFilterBuilder_EmptyBuild(t *testing.T) {
	opts := Filter().Build()
	assert.Empty(t, opts)
}

func TestFilterBuilder_ChainImmutability(t *testing.T) {
	db := openDryRunDB(t)
	base := Filter().Eq(int64(1), "a")
	branch1 := Filter().Eq(int64(1), "a").Eq(int64(2), "b")
	branch2 := Filter().Eq(int64(1), "a").Eq(int64(3), "c")

	_, sql0 := buildSQL(db, base.Build())
	_, sql1 := buildSQL(db, branch1.Build())
	_, sql2 := buildSQL(db, branch2.Build())

	assert.Contains(t, sql0, "a = 1")
	assert.NotContains(t, sql0, "b")
	assert.Contains(t, sql1, "b = 2")
	assert.Contains(t, sql2, "c = 3")
}

func TestFilterBuilder_CommunityPaginationNilSafe(t *testing.T) {
	db := openDryRunDB(t)
	var p *community.Paging
	opts := Filter().
		Eq(int64(1), "id").
		Option(communityPaginationNil(p)).
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.Contains(t, realSQL, "id = 1")
	assert.NotContains(t, strings.ToLower(realSQL), "limit")
}

func communityPaginationNil(p *community.Paging) QueryOption {
	return Pagination(p)
}
