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
		Eq("created_at", int64(42)).
		Eq("name", "alice").
		Build()
	_, realSQL := buildSQL(db, opts)
	t.Logf("Real SQL: %s", realSQL)
	assert.Contains(t, realSQL, "created_at = 42")
	assert.Contains(t, realSQL, `name = "alice"`)
}

func TestFilterBuilder_ZeroValuesSkipped(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Eq("status", int64(0)).
		Eq("name", "").
		Eq("id", int64(5)).
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
		Eq("a", int64(1)).
		Neq("b", int64(2)).
		Gt("c", int64(3)).
		Lt("d", int64(4)).
		Gte("e", int64(5)).
		Lte("f", int64(6)).
		Like("g", "kw").
		ILike("h", "kw2").
		In("i", []int64{7, 8}).
		NotIn("j", []int64{9}).
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
		Between("created_at", int64(100), int64(200)).
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
		Between("created_at", int64(0), int64(200)).
		Eq("id", int64(1)).
		Build()
	_, realSQL := buildSQL(db, opts)
	t.Logf("Real SQL: %s", realSQL)
	assert.NotContains(t, realSQL, "BETWEEN")
	assert.Contains(t, realSQL, "id = 1")
}

func TestFilterBuilder_BetweenNilSkipped(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Between("created_at", nil, int64(200)).
		Eq("id", int64(1)).
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.NotContains(t, realSQL, "BETWEEN")
	assert.Contains(t, realSQL, "id = 1")
}

func TestFilterBuilder_Must(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Must("mchid", int64(0)).
		Eq("status", int64(0)).
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
		Eq("id", int64(1)).
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.NotContains(t, realSQL, "mchid")
	assert.Contains(t, realSQL, "id = 1")
}

func TestFilterBuilder_OptionAndOptions(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Eq("status", int64(1)).
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
		Eq("id", int64(1)).
		Option(nil).
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.Contains(t, realSQL, "id = 1")
}

func TestFilterBuilder_OptionsNilSafe(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Eq("id", int64(1)).
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
		Eq("id", int64(1)).
		Option(Pagination(p)).
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.Contains(t, realSQL, "id = 1")
	assert.NotContains(t, strings.ToLower(realSQL), "limit")
}

func TestFilterBuilder_OrderEmptySafe(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Eq("id", int64(1)).
		Option(Order("")).
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.Contains(t, realSQL, "id = 1")
	assert.NotContains(t, strings.ToLower(realSQL), "order by")
}

func TestFilterBuilder_NilValInEq(t *testing.T) {
	db := openDryRunDB(t)
	opts := Filter().
		Eq("status", nil).
		Eq("id", int64(1)).
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.NotContains(t, realSQL, "status")
	assert.Contains(t, realSQL, "id = 1")
}

func TestFilterBuilder_MatchesStandaloneFunctions(t *testing.T) {
	db := openDryRunDB(t)
	_, fromBuilder := buildSQL(db, Filter().Eq("created_at", int64(42)).Build())
	_, fromFn := buildSQL(db, []QueryOption{FilterEq("created_at", int64(42))})
	assert.Equal(t, fromBuilder, fromFn)
}

func TestFilterBuilder_EmptyBuild(t *testing.T) {
	opts := Filter().Build()
	assert.Empty(t, opts)
}

func TestFilterBuilder_ChainImmutability(t *testing.T) {
	db := openDryRunDB(t)
	base := Filter().Eq("a", int64(1))
	branch1 := Filter().Eq("a", int64(1)).Eq("b", int64(2))
	branch2 := Filter().Eq("a", int64(1)).Eq("c", int64(3))

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
		Eq("id", int64(1)).
		Option(communityPaginationNil(p)).
		Build()
	_, realSQL := buildSQL(db, opts)
	assert.Contains(t, realSQL, "id = 1")
	assert.NotContains(t, strings.ToLower(realSQL), "limit")
}

func communityPaginationNil(p *community.Paging) QueryOption {
	return Pagination(p)
}
