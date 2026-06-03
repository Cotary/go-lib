//go:build ignore

// 本文件为使用示例，仅供参考，不可直接编译运行

package model

import (
	"myproject/config"

	"go-lib/dao/gormDB"
)

// 表名常量，集中管理避免硬编码
const (
	TableNameOrder = "orders"
	TableNameUser  = "users"
)

// DBDriver 全局数据库驱动实例，供所有 Model 的 CRUD 方法使用
var DBDriver *gormDB.GormDrive

// Init 初始化数据库连接，应在服务启动时调用。
// MustNewGorm 失败时 panic，适合 init 阶段使用。
func Init() {
	DBDriver = gormDB.MustNewGorm(config.Config.DB)
}
