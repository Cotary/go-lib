package gormDB

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestRow 是测试专用的模型结构体，映射到表 t，字段精简以覆盖主要查询场景
type TestRow struct {
	ID        int64
	Name      string
	Status    int64
	TenantID  int64
	Age       int64
	ParentID  *int64
	CreatedAt int64
}

// TableName 返回固定的 SQL 表名
func (TestRow) TableName() string { return "t" }

// openDryRunDB 创建一个 SQL 不真正执行的 GORM 实例（DryRun 模式），
// 适用于只需验证 SQL 生成而不需要 docker/sqlite 真实库的场景
func openDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("open dry run db: %v", err)
	}
	return db
}

// buildSQL 对 Scope 应用到 db，执行 Find 获取带占位符的 SQL 和展开参数后的实际 SQL。
// 仅用于测试中 realSQL 的断言验证
func buildSQL(db *gorm.DB, scopes ...Scope) (placeholder string, realSQL string) {
	tx := db.Scopes(scopes...).Find(&[]TestRow{})
	placeholder = tx.Statement.SQL.String()
	realSQL = db.Dialector.Explain(placeholder, tx.Statement.Vars...)
	return
}
