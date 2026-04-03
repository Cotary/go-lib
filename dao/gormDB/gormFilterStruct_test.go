package gormDB

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
	Eq         int64   `filter:"created_at,eq"`
	Ne         int64   `filter:"created_at,ne"`
	Gt         int64   `filter:"created_at,gt"`
	Lt         int64   `filter:"created_at,lt"`
	Ge         int64   `filter:"created_at,gte"`
	Le         int64   `filter:"created_at,lte"`
	Like       string  `filter:"name,like"`
	ILike      string  `filter:"description,ilike"`
	In         []int64 `filter:"id,in"`
	NotIn      []int64 `filter:"id,not_in"`
	TimeZeroLe int64   `filter:"timezero,lte"`
	TimeGe     string  `filter:"time,gte"`
	TimeLe     string  `filter:"time,lte"`
}

func TestAllOpsFilterSQL(t *testing.T) {
	db := openDryRunDB(t)

	f := AllOpsFilter{
		Eq: 10, Ne: 11, Gt: 20, Lt: 30, Ge: 40, Le: 50,
		Like: "foo", ILike: "bar",
		In: []int64{1, 2, 3}, NotIn: []int64{4, 5},
		TimeZeroLe: 20,
		TimeGe:     "30", TimeLe: "40",
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
	assert.Contains(t, realSQL, `time >=`)
	assert.Contains(t, realSQL, `time <=`)
	assert.Contains(t, realSQL, `timezero <= 20`)
}

// --- 多列 OR；非法操作符（含 &、|）静默跳过 ---

type MultiColFilter struct {
	Keyword string `filter:"name|description,like"`
	RangeGe int64  `filter:"created_at,ge"`
	RangeLe int64  `filter:"created_at,le"`
}

func TestMultiColFilterSQL(t *testing.T) {
	db := openDryRunDB(t)

	f := MultiColFilter{
		Keyword: "kw", RangeGe: 100, RangeLe: 200,
	}
	opts := BuildQueryOptions(f, nil)
	_, realSQL := buildSQL(db, opts)

	t.Logf("Real SQL:\n%s", realSQL)

	assert.Contains(t, realSQL, `name like`)
	assert.Contains(t, realSQL, `description like`)
	assert.Contains(t, realSQL, `OR`)
	assert.Contains(t, realSQL, `created_at >= 100`)
	assert.Contains(t, realSQL, `created_at <= 200`)
}

type IllegalOpFilter struct {
	Bad int64 `filter:"created_at,ge&le"`
}

func TestIllegalOpFilterSkipped(t *testing.T) {
	db := openDryRunDB(t)
	f := IllegalOpFilter{Bad: 1}
	opts := BuildQueryOptions(f, nil)
	_, realSQL := buildSQL(db, opts)
	assert.NotContains(t, strings.ToLower(realSQL), "created_at")
}

func TestFilterEqMatchesBuildQueryOptions(t *testing.T) {
	db := openDryRunDB(t)
	_, fromTag := buildSQL(db, BuildQueryOptions(struct {
		V int64 `filter:"created_at,eq"`
	}{V: 42}, nil))
	_, fromFn := buildSQL(db, []QueryOption{FilterEq("created_at", int64(42))})
	assert.Equal(t, fromTag, fromFn)
}

func TestFilterEqPointerZeroStillFilters(t *testing.T) {
	db := openDryRunDB(t)
	z := int64(0)
	p := &z
	_, realSQL := buildSQL(db, []QueryOption{FilterEq("status", p)})
	assert.Contains(t, realSQL, `status = 0`)
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

// BuildQueryOptions 应固定为：Where → 分页 → Order，与结构体字段顺序无关；最终 SQL 中 ORDER BY 须在 LIMIT 之前。
func TestBuildQueryOptions_PagingWhereOrderClauseOrder(t *testing.T) {
	db := openDryRunDB(t)

	type OrderedListReq struct {
		Paging community.Paging
		Order  community.Order
		Name   string `filter:"name,like"`
	}

	bind := map[string]string{
		"created_at": "created_at {order_type}",
	}
	req := OrderedListReq{
		Paging: community.Paging{Page: 1, PageSize: 10},
		Order:  community.Order{OrderField: "created_at", OrderType: "desc"},
		Name:   "kw",
	}

	placeholder, _ := buildSQL(db, BuildQueryOptions(req, bind))
	lower := strings.ToLower(placeholder)

	t.Logf("Placeholder SQL: %s", placeholder)

	iWhere := strings.Index(lower, "where")
	iOrder := strings.Index(lower, "order by")
	iLimit := strings.Index(lower, "limit")
	assert.GreaterOrEqual(t, iWhere, 0, "expected WHERE")
	assert.Greater(t, iOrder, iWhere, "ORDER BY should follow WHERE")
	assert.Greater(t, iLimit, iOrder, "LIMIT should follow ORDER BY")
}

func TestBuildQueryOptions_NilPagingOrderPtrSkipped(t *testing.T) {
	db := openDryRunDB(t)
	type row struct {
		Paging *community.Paging
		Order  *community.Order
		Name   string `filter:"name,eq"`
	}
	f := row{Name: "x"}
	opts := BuildQueryOptions(f, nil)
	_, sql := buildSQL(db, opts)
	assert.Contains(t, sql, `name =`)
	assert.NotContains(t, strings.ToLower(sql), "limit")
	assert.NotContains(t, strings.ToLower(sql), "order by")
}

func TestBuildQueryOptions_PagingPtrNonNilUsesPagination(t *testing.T) {
	db := openDryRunDB(t)
	p := &community.Paging{Page: 1, PageSize: 7}
	f := struct {
		Paging *community.Paging
	}{Paging: p}
	opts := BuildQueryOptions(f, nil)
	_, sql := buildSQL(db, opts)
	assert.Contains(t, strings.ToLower(sql), "limit")
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

// ---------------------------------------------------------------------------
// nil 安全测试
// ---------------------------------------------------------------------------

func TestBuildQueryOptions_NilCriteria(t *testing.T) {
	opts := BuildQueryOptions(nil, nil)
	assert.Empty(t, opts)
}

func TestBuildQueryOptions_NilPointerCriteria(t *testing.T) {
	type F struct {
		Name string `filter:"name,eq"`
	}
	var p *F
	opts := BuildQueryOptions(p, nil)
	assert.Empty(t, opts)
}

func TestPagination_NilNoPanic(t *testing.T) {
	db := openDryRunDB(t)
	opt := Pagination(nil)
	_, sql := buildSQL(db, []QueryOption{opt})
	assert.NotContains(t, strings.ToLower(sql), "limit")
}

func TestPaging_NilNoPanic(t *testing.T) {
	db := openDryRunDB(t)
	opt := Paging(nil)
	_, sql := buildSQL(db, []QueryOption{opt})
	assert.NotContains(t, strings.ToLower(sql), "limit")
}

func TestOrder_EmptyStringNoop(t *testing.T) {
	db := openDryRunDB(t)
	opt := Order("")
	_, sql := buildSQL(db, []QueryOption{opt})
	assert.NotContains(t, strings.ToLower(sql), "order by")
}
