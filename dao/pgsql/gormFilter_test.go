package pgsql

import (
	"context"
	"strings"
	"testing"

	"github.com/Cotary/go-lib/common/community"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

type FilterUser struct {
	ID          int64
	Name        string
	Description string
	CreatedAt   int64
}

func (FilterUser) TableName() string {
	return "ttt"
}

func openDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	return db
}

func buildSQL(db *gorm.DB, opts []QueryOption) (placeholder string, realSQL string) {
	var scopes []func(*gorm.DB) *gorm.DB
	for _, opt := range opts {
		scopes = append(scopes, opt)
	}
	stmt := db.Scopes(scopes...).Find(&[]FilterUser{})
	placeholder = stmt.Statement.SQL.String()
	realSQL = db.Dialector.Explain(placeholder, stmt.Statement.Vars...)
	return
}

// --- 单字段操作符 ---

type AllOpsFilter struct {
	Eq               int64   `filter:"created_at,eq"`
	Ne               int64   `filter:"created_at,ne"`
	Gt               int64   `filter:"created_at,gt"`
	Lt               int64   `filter:"created_at,lt"`
	Ge               int64   `filter:"created_at,ge"`
	Le               int64   `filter:"created_at,le"`
	Like             string  `filter:"name,like"`
	ILike            string  `filter:"description,ilike"`
	In               []int64 `filter:"id,in"`
	NotIn            []int64 `filter:"id,not_in"`
	BetweenStartZero int64   `filter:"timezero,bs.1"`
	BetweenEndZero   int64   `filter:"timezero,be.1"`
	BetweenStart     string  `filter:"time,bs"`
	BetweenEnd       string  `filter:"time,be"`
}

func TestAllOpsFilterSQL(t *testing.T) {
	db := openDryRunDB(t)

	f := AllOpsFilter{
		Eq: 10, Ne: 11, Gt: 20, Lt: 30, Ge: 40, Le: 50,
		Like: "foo", ILike: "bar",
		In: []int64{1, 2, 3}, NotIn: []int64{4, 5},
		BetweenEndZero: 20,
		BetweenStart:   "30", BetweenEnd: "40",
	}
	opts := BuildQueryOptions(f, nil)
	placeholder, realSQL := buildSQL(db, opts)

	t.Logf("Placeholder SQL:\n%s", placeholder)
	t.Logf("Real SQL:\n%s", realSQL)

	assert.Contains(t, realSQL, `created_at = 10`)
	assert.Contains(t, realSQL, `created_at <> 11`)
	assert.Contains(t, realSQL, `created_at > 20`)
	assert.Contains(t, realSQL, `created_at < 30`)
	assert.Contains(t, realSQL, `created_at >= 40`)
	assert.Contains(t, realSQL, `created_at <= 50`)
	assert.Contains(t, realSQL, `name like`)
	assert.Contains(t, realSQL, `description ilike`)
	assert.Contains(t, realSQL, `id IN`)
	assert.Contains(t, realSQL, `id NOT IN`)
	assert.Contains(t, realSQL, `time BETWEEN`)
	assert.Contains(t, realSQL, `timezero <= 20`)
}

// --- 组合 AND/OR 操作符 ---

type ComboFilter struct {
	RangeAnd    int64 `filter:"created_at,ge&le"`
	RangeOr     int64 `filter:"created_at,ge|le"`
	MultiColAnd int64 `filter:"name|description,like&eq|<>"`
	MultiColOr  int64 `filter:"name|description,like|eq"`
}

func TestComboFilterSQL(t *testing.T) {
	db := openDryRunDB(t)

	f := ComboFilter{
		RangeAnd: 100, RangeOr: 200, MultiColAnd: 5, MultiColOr: 6,
	}
	opts := BuildQueryOptions(f, nil)
	placeholder, realSQL := buildSQL(db, opts)

	t.Logf("Placeholder SQL:\n%s", placeholder)
	t.Logf("Real SQL:\n%s", realSQL)

	assert.Contains(t, realSQL, `created_at >= 100`)
	assert.Contains(t, realSQL, `created_at <= 100`)
	assert.Contains(t, realSQL, `AND`)
	assert.Contains(t, realSQL, `OR`)
}

// --- 嵌套结构体 ---

type ChildFilter struct {
	EmailEq   string  `filter:"description,eq"`
	EmailLike string  `filter:"description,like"`
	IDIn      []int64 `filter:"id,in"`
}

type ParentFilter struct {
	NameLike string `filter:"name,like"`
	ChildFilter
}

func TestNestedFilterSQL(t *testing.T) {
	db := openDryRunDB(t)

	f := ParentFilter{
		NameLike: "alice",
		ChildFilter: ChildFilter{
			EmailEq:   "bob",
			EmailLike: "bob",
			IDIn:      []int64{7, 8},
		},
	}
	opts := BuildQueryOptions(f, nil)
	placeholder, realSQL := buildSQL(db, opts)

	t.Logf("Placeholder SQL:\n%s", placeholder)
	t.Logf("Real SQL:\n%s", realSQL)

	assert.Contains(t, realSQL, `name like`)
	assert.Contains(t, realSQL, `description =`)
	assert.Contains(t, realSQL, `description like`)
	assert.Contains(t, realSQL, `id IN`)
}

// --- 排序 ---

func TestOrderOptionSQL(t *testing.T) {
	db := openDryRunDB(t)

	filter := struct {
		Order community.Order
	}{
		Order: community.Order{
			OrderField: "created_at,name",
			OrderType:  "desc,asc",
		},
	}

	bind := map[string]string{
		"created_at": "created_at {order_type}",
		"name":       "name",
	}
	opts := BuildQueryOptions(filter, bind)
	placeholder, _ := buildSQL(db, opts)

	t.Logf("Placeholder SQL: %s", placeholder)

	assert.Contains(t, strings.ToLower(placeholder), "order by")
	assert.Contains(t, placeholder, "created_at desc")
}

func TestComplexMultiFieldOrderSQL(t *testing.T) {
	db := openDryRunDB(t)

	filter := struct {
		Order community.Order
	}{
		Order: community.Order{
			OrderField: "search_field,created_at",
			OrderType:  "asc,desc",
		},
	}

	bind := map[string]string{
		"search_field": "name,description {order_type}",
		"created_at":   "created_at",
	}
	opts := BuildQueryOptions(filter, bind)
	placeholder, _ := buildSQL(db, opts)

	t.Logf("Placeholder SQL:\n%s", placeholder)

	assert.Contains(t, placeholder, "name,description asc")
}

// --- 无效 tag 不 panic ---

func TestBuildQueryOptions_InvalidTag(t *testing.T) {
	type BadFilter struct {
		NoComma string `filter:"name"`
	}
	f := BadFilter{NoComma: "test"}
	opts := BuildQueryOptions(f, nil)
	assert.Empty(t, opts)
}

// --- 零值字段跳过 ---

func TestBuildQueryOptions_ZeroValues(t *testing.T) {
	type ZeroFilter struct {
		Name string `filter:"name,eq"`
		Age  int64  `filter:"age,gt"`
	}
	f := ZeroFilter{}
	opts := BuildQueryOptions(f, nil)
	assert.Empty(t, opts)
}

// --- ListT 辅助函数 ---

func ListT[T any](ctx context.Context, db *gorm.DB, opts ...QueryOption) (res []T, stmt *gorm.DB) {
	q := db.WithContext(ctx).Model(new(T))
	for _, opt := range opts {
		q = opt(q)
	}
	stmt = q.Find(&res)
	return res, stmt
}

func TestListTWithStructAndClosure(t *testing.T) {
	db := openDryRunDB(t)

	type DynFilter struct {
		NameLike  string `filter:"name,like"`
		CreatedGT int64  `filter:"created_at,gt"`
	}
	f := DynFilter{NameLike: "ali", CreatedGT: 5}

	opts := BuildQueryOptions(f, nil)
	opts = append(opts,
		func(db *gorm.DB) *gorm.DB {
			return db.Where("created_at <= ?", 15)
		},
		func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc")
		},
	)

	placeholder, realSQL := buildSQL(db, opts)

	t.Logf("Placeholder SQL: %s", placeholder)
	t.Logf("Real SQL: %s", realSQL)

	assert.Contains(t, realSQL, `name like`)
	assert.Contains(t, realSQL, `created_at > 5`)
	assert.Contains(t, realSQL, `created_at <= 15`)
	assert.Contains(t, strings.ToLower(placeholder), "order by")
}

func TestListTWithModel(t *testing.T) {
	db := openDryRunDB(t)

	type DynFilter struct {
		NameLike  string `filter:"name,like"`
		CreatedGT int64  `filter:"created_at,gt"`
	}
	f := DynFilter{NameLike: "ali", CreatedGT: 5}

	opts := BuildQueryOptions(f, nil)

	ctx := context.Background()
	dbModel := db.WithContext(ctx)

	var req struct {
		Paging community.Paging
	}
	opts = append(opts, Pagination(&req.Paging))

	users, stmt := ListT[FilterUser](ctx, dbModel, opts...)

	placeholder := stmt.Statement.SQL.String()
	t.Logf("Placeholder SQL: %s", placeholder)
	t.Logf("Users: %v", users)

	assert.Contains(t, placeholder, "name like")
	assert.Contains(t, placeholder, "created_at > ?")
}

func TestListTWithModelMap(t *testing.T) {
	db := openDryRunDB(t)

	type DynFilter struct {
		NameLike  string `filter:"name,like"`
		CreatedGT int64  `filter:"created_at,gt"`
	}
	f := DynFilter{NameLike: "ali", CreatedGT: 5}

	opts := BuildQueryOptions(f, nil)

	ctx := context.Background()
	dbModel := db.WithContext(ctx)

	var req struct {
		Paging community.Paging
	}
	opts = append(opts, Table("vvv"), Pagination(&req.Paging))

	type UserMap map[string]interface{}
	users, stmt := ListT[UserMap](ctx, dbModel, opts...)

	placeholder := stmt.Statement.SQL.String()
	t.Logf("Placeholder SQL: %s", placeholder)
	t.Logf("Users: %v", users)

	assert.Contains(t, placeholder, "`vvv`")
}
