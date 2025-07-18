package psql

import (
	"context"
	"github.com/Cotary/go-lib/common/community"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// User 是测试用模型
type User struct {
	ID          int64
	Name        string
	Description string
	CreatedAt   int64
}

func (User) TableName() string {
	return "ttt"
}

// openDryRunDB 返回一个开启 DryRun 的 SQLite 连接
func openDryRunDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	return db
}

// 1. 全部单字段 & 多值操作符
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

	BetweenStart string `filter:"time,bs"`
	BetweenEnd   string `filter:"time,be"`
}

func TestAllOpsFilterSQL(t *testing.T) {
	db := openDryRunDB(t)

	f := AllOpsFilter{
		Eq:    10,
		Ne:    11,
		Gt:    20,
		Lt:    30,
		Ge:    40,
		Le:    50,
		Like:  "foo",
		ILike: "bar",
		In:    []int64{1, 2, 3},
		NotIn: []int64{4, 5},
		//BetweenStartZero: 10,
		BetweenEndZero: 20,
		BetweenStart:   "30",
		BetweenEnd:     "40",
	}
	opts := BuildQueryOptions(f, nil)

	// 转为 scopes
	var scopes []func(*gorm.DB) *gorm.DB
	for _, opt := range opts {
		scopes = append(scopes, opt)
	}
	stmt := db.Scopes(scopes...).Find(&[]User{})

	placeholder := stmt.Statement.SQL.String()
	real := db.Dialector.Explain(placeholder, stmt.Statement.Vars...)

	t.Logf("=== TestAllOpsFilterSQL ===")
	t.Logf("Placeholder SQL:\n%s", placeholder)
	t.Logf("Real SQL:\n%s", real)
}

// 2. 组合 AND/OR 操作符 & 多列 AND/OR
type ComboFilter struct {
	// AND 组合：ge&le
	RangeAnd int64 `filter:"created_at,ge&le"`
	// OR 组合：ge|le
	RangeOr int64 `filter:"created_at,ge|le"`
	// 多列 AND：like&eq
	MultiColAnd int64 `filter:"name|description,like&eq|<>"`
	// 多列 OR：like|eq
	MultiColOr int64 `filter:"name|description,like|eq"`
}

func TestComboFilterSQL(t *testing.T) {
	db := openDryRunDB(t)

	f := ComboFilter{
		RangeAnd:    100,
		RangeOr:     200,
		MultiColAnd: 5,
		MultiColOr:  6,
	}
	opts := BuildQueryOptions(f, nil)

	var scopes []func(*gorm.DB) *gorm.DB
	for _, opt := range opts {
		scopes = append(scopes, opt)
	}
	stmt := db.Scopes(scopes...).Find(&[]User{})

	placeholder := stmt.Statement.SQL.String()
	real := db.Dialector.Explain(placeholder, stmt.Statement.Vars...)

	t.Logf("=== TestComboFilterSQL ===")
	t.Logf("Placeholder SQL:\n%s", placeholder)
	t.Logf("Real SQL:\n%s", real)
}

// 3. 嵌套结构体 + 新增操作符
type ChildFilter struct {
	EmailEq   string  `filter:"description,eq"`
	EmailLike string  `filter:"description,like"`
	IDIn      []int64 `filter:"id,in"`
}

type ParentFilter struct {
	NameLike    string `filter:"name,like"`
	ChildFilter        // 嵌套
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

	var scopes []func(*gorm.DB) *gorm.DB
	for _, opt := range opts {
		scopes = append(scopes, opt)
	}
	stmt := db.Scopes(scopes...).Find(&[]User{})

	placeholder := stmt.Statement.SQL.String()
	real := db.Dialector.Explain(placeholder, stmt.Statement.Vars...)

	t.Logf("=== TestNestedFilterSQL ===")
	t.Logf("Placeholder SQL:\n%s", placeholder)
	t.Logf("Real SQL:\n%s", real)
}

func TestOrderOptionSQL(t *testing.T) {
	db := openDryRunDB(t)

	// 只测试 Order
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

	// 转为 scopes
	var scopes []func(*gorm.DB) *gorm.DB
	for _, opt := range opts {
		scopes = append(scopes, opt)
	}
	stmt := db.Scopes(scopes...).Find(&[]User{})

	placeholder := stmt.Statement.SQL.String()
	real := db.Dialector.Explain(placeholder, stmt.Statement.Vars...)

	t.Logf("=== TestOrderOptionSQL ===")
	t.Logf("Placeholder SQL: %s", placeholder)
	t.Logf("Real SQL: %s", real)
}

func TestComplexMultiFieldOrderSQL(t *testing.T) {
	db := openDryRunDB(t)

	// 只测试多字段排序 + 自定义 {order_type}
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

	// 转成 scopes
	var scopes []func(*gorm.DB) *gorm.DB
	for _, opt := range opts {
		scopes = append(scopes, opt)
	}

	stmt := db.Scopes(scopes...).Find(&[]User{})
	placeholder := stmt.Statement.SQL.String()
	real := db.Dialector.Explain(placeholder, stmt.Statement.Vars...)

	t.Logf("=== TestComplexMultiFieldOrderSQL ===")
	t.Logf("Placeholder SQL:\n%s", placeholder)
	t.Logf("Real SQL:\n%s", real)
}

// TestListTWithStructAndClosure 演示使用 ListT，并在 opts 中同时使用结构体 tag 过滤和闭包自定义过滤/排序
func TestListTWithStructAndClosure(t *testing.T) {
	db := openDryRunDB(t)
	// 3. 定义动态过滤结构体
	type DynFilter struct {
		NameLike  string `filter:"name,like"`     // 模糊 name
		CreatedGT int64  `filter:"created_at,gt"` // created_at > ?
	}
	f := DynFilter{
		NameLike:  "ali",
		CreatedGT: 5,
	}

	// 4. 从结构体生成 QueryOption
	opts := BuildQueryOptions(f, nil)

	// 5. 在 opts 后追加一个闭包：只保留 CreatedAt <= 15，并按 CreatedAt DESC 排序
	opts = append(opts,
		func(db *gorm.DB) *gorm.DB {
			return db.Where("created_at <= ?", 15)
		},
		func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc")
		},
	)

	// 6. 调用泛型 ListT
	//ctx := context.Background()
	//users, err := ListT[User](ctx, db, opts...)
	//if err != nil {
	//	t.Fatalf("ListT returned error: %v", err)
	//}

	// 转为 scopes
	var scopes []func(*gorm.DB) *gorm.DB
	for _, opt := range opts {
		scopes = append(scopes, opt)
	}
	stmt := db.Scopes(scopes...).Find(&[]User{})

	placeholder := stmt.Statement.SQL.String()
	real := db.Dialector.Explain(placeholder, stmt.Statement.Vars...)

	t.Logf("=== TestOrderOptionSQL ===")
	t.Logf("Placeholder SQL: %s", placeholder)
	t.Logf("Real SQL: %s", real)
}

func TestListTWithModel(t *testing.T) {
	db := openDryRunDB(t)
	// 3. 定义动态过滤结构体
	type DynFilter struct {
		NameLike  string `filter:"name,like"`     // 模糊 name
		CreatedGT int64  `filter:"created_at,gt"` // created_at > ?
	}
	f := DynFilter{
		NameLike:  "ali",
		CreatedGT: 5,
	}

	// 4. 从结构体生成 QueryOption
	opts := BuildQueryOptions(f, nil)

	// 6. 调用泛型 ListT
	ctx := context.Background()

	dbModel := db.WithContext(ctx)

	var req struct {
		Paging community.Paging
	}
	opts = append(opts,
		WithPagination(&req.Paging),
	)

	users, stmt := ListT[User](ctx, dbModel, opts...)

	placeholder := stmt.Statement.SQL.String()
	real := db.Dialector.Explain(placeholder, stmt.Statement.Vars...)

	t.Logf("=== TestOrderOptionSQL ===")

	t.Logf("Stmt Error: %s", stmt.Error)
	t.Logf("Placeholder SQL: %s", placeholder)
	t.Logf("Real SQL: %s", real)
	t.Logf("Users: %v", users)
}

func TestListTWithModelMap(t *testing.T) {
	db := openDryRunDB(t)
	// 3. 定义动态过滤结构体
	type DynFilter struct {
		NameLike  string `filter:"name,like"`     // 模糊 name
		CreatedGT int64  `filter:"created_at,gt"` // created_at > ?
	}
	f := DynFilter{
		NameLike:  "ali",
		CreatedGT: 5,
	}

	// 4. 从结构体生成 QueryOption
	opts := BuildQueryOptions(f, nil)

	// 6. 调用泛型 ListT
	ctx := context.Background()

	dbModel := db.WithContext(ctx)

	var req struct {
		Paging community.Paging
	}
	opts = append(opts,
		WithTable("vvv"),
		WithPagination(&req.Paging),
	)

	type UserMap map[string]interface{}
	users, stmt := ListT[UserMap](ctx, dbModel, opts...)

	placeholder := stmt.Statement.SQL.String()
	real := db.Dialector.Explain(placeholder, stmt.Statement.Vars...)

	t.Logf("=== TestOrderOptionSQL ===")

	t.Logf("Stmt Error: %s", stmt.Error)
	t.Logf("Placeholder SQL: %s", placeholder)
	t.Logf("Real SQL: %s", real)
	t.Logf("Users: %v", users)
}
func ListT[T any](ctx context.Context, db *gorm.DB, opts ...QueryOption) (res []T, stmt *gorm.DB) {

	q := db.WithContext(ctx).Model(new(T))
	for _, opt := range opts {
		q = opt(q)
	}
	stmt = q.Find(&res)
	return res, stmt
}
