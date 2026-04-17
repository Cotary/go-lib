// Package gormDB 是 GORM 的精简增强层，遵循「增强 GORM 而非替代 GORM」的设计理念。
//
// 设计原则：
//
//  1. 不造轮子：能用 GORM 原生 API 表达的一律不封装
//  2. 增强而非替代：用户依然直接写 GORM，仅在有增量价值的地方调用本包 helper
//  3. 与 GORM Scopes 无缝对接：所有条件辅助函数都是 Scope 类型
//     即 func(*gorm.DB) *gorm.DB，可直接传给 db.Scopes(...)
//
// 主要功能模块：
//
//   - 零值 / nil 智能跳过的 Where（WhereIf）
//   - 强制生效（含零值和 nil 折回零值）的 Where（WhereAlways）
//   - NULL 友好的 Where（WhereNullable / IsNull / IsNotNull）
//   - 自动 count + Total 回写的分页（Paginate）
//   - 白名单驱动的安全排序（OrderWhitelist）
//   - 基于 struct tag 的一键装配过滤（ApplyFilter）
//   - 闭包式事务传播（CtxTransaction / WithContext）
//   - Upsert 与复合保存（Save / QueryAndSave）
//
// 完整使用说明参见 README.md。
package gormDB

import (
	"context"
	"fmt"
	"time"

	log2 "github.com/Cotary/go-lib/log"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// GormConfig 是创建 GormDrive 所需的数据库配置。
//
// Dsn 为数据库连接字符串，索引 0 为主连接的 DSN，后续预留用于读写分离等扩展。
type GormConfig struct {
	Driver string   `mapstructure:"driver" yaml:"driver"`
	Dsn    []string `mapstructure:"dsn" yaml:"dsn"`

	ConnMaxLife int `mapstructure:"connMaxLife" yaml:"connMaxLife"` // 连接最大存活时间（秒），0 表示不限制
	ConnMaxIdle int `mapstructure:"connMaxIdle" yaml:"connMaxIdle"` // 空闲连接最大存活时间（秒），0 表示不限制
	MaxOpens    int `mapstructure:"maxOpens" yaml:"maxOpens"`       // 最大打开连接数，0 表示不限制
	MaxIdles    int `mapstructure:"maxIdles" yaml:"maxIdles"`       // 最大空闲连接数，默认 2

	LogDir        string `mapstructure:"log_dir" yaml:"log_dir"`
	LogLevel      string `mapstructure:"log_level" yaml:"log_level"`           // silent / error / warn / info
	SlowThreshold int64  `mapstructure:"slow_threshold" yaml:"slow_threshold"` // 慢 SQL 阈值（毫秒）
	LogSaveDay    int64  `mapstructure:"log_save_day" yaml:"log_save_day"`     // 日志保留天数
}

// GormDrive 封装了单个数据库连接实例及其日志记录器。
//
// ID 为 uuid 标识，用作 context key 的一部分，支持同进程内多个数据库实例独立管理事务。
type GormDrive struct {
	ID     string
	Logger *GormLogger
	db     *gorm.DB
}

// DB 返回底层的 *gorm.DB 实例。
//
// 注意：通过此方法获取的实例不会自动注入 context 中的事务。
// 业务代码推荐使用 WithContext(ctx) 以自动接入事务上下文。
func (g *GormDrive) DB() *gorm.DB {
	return g.db
}

// Health 执行数据库健康检查，验证连接池 / 网络是否正常
func (g *GormDrive) Health(ctx context.Context) error {
	if g.db == nil {
		return fmt.Errorf("gormDB: db is nil")
	}
	sqlDB, err := g.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Close 关闭底层数据库连接，释放连接池资源。
// 应在应用退出时调用，调用后 GormDrive 实例不可再使用。
func (g *GormDrive) Close() error {
	if g.db == nil {
		return nil
	}
	sqlDB, err := g.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// handleConfig 为 GormConfig 中未设置的字段填充合理的默认值
func handleConfig(config *GormConfig) {
	if config.LogDir == "" {
		config.LogDir = "./logs/gorm/"
	}
	if config.LogLevel == "" {
		config.LogLevel = "info"
	}
	if config.LogSaveDay == 0 {
		config.LogSaveDay = 30
	}
	if config.SlowThreshold == 0 {
		config.SlowThreshold = 1000
	}
	if config.MaxOpens == 0 {
		config.MaxOpens = 100
	}
	if config.MaxIdles == 0 {
		config.MaxIdles = 20
	}
	if config.ConnMaxLife == 0 {
		config.ConnMaxLife = 3600
	}
	if config.ConnMaxIdle == 0 {
		config.ConnMaxIdle = 600
	}
}

func getLogLevelEnum(level string) logger.LogLevel {
	switch level {
	case "silent":
		return logger.Silent
	case "error":
		return logger.Error
	case "warn":
		return logger.Warn
	default:
		return logger.Info
	}
}

func getDriver(driver string, dsn []string) gorm.Dialector {
	if len(dsn) == 0 {
		return nil
	}
	switch driver {
	case "sqlite":
		return sqlite.Open(dsn[0])
	case "mysql":
		return mysql.Open(dsn[0])
	case "postgres":
		return postgres.Open(dsn[0])
	default:
		return nil
	}
}

// NewGorm 创建并初始化 GormDrive 实例。
//
// 默认开启 SkipDefaultTransaction 和 PrepareStmt 以获得更好的性能。
// 如需自定义 GORM 配置项，目前需修改 NewGorm 内部代码。
// 后续可通过 Functional Options 模式扩展，保持对外 API 向后兼容。
func NewGorm(config *GormConfig) (*GormDrive, error) {
	handleConfig(config)

	logConfig := log2.Config{
		Driver:     log2.DriverZap,
		Level:      "info",
		Path:       config.LogDir,
		FileSuffix: ".log",
		MaxAge:     config.LogSaveDay,
		FileName:   "gorm-logger",
		MaxSize:    10,
		Compress:   false,
		ShowFile:   false,
		Format:     log2.FormatText,
	}
	writer := log2.NewLogger(&logConfig)
	newLogger := NewGormLogger(
		NewGormLogWriter(writer),
		logger.Config{
			SlowThreshold:             time.Duration(config.SlowThreshold) * time.Millisecond,
			LogLevel:                  getLogLevelEnum(config.LogLevel),
			IgnoreRecordNotFoundError: false,
			ParameterizedQueries:      false,
			Colorful:                  false,
		},
	)

	dialector := getDriver(config.Driver, config.Dsn)
	if dialector == nil {
		return nil, fmt.Errorf("unsupported database driver: %s", config.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
		Logger:                 newLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}
	sqlDB.SetMaxIdleConns(config.MaxIdles)
	sqlDB.SetMaxOpenConns(config.MaxOpens)
	sqlDB.SetConnMaxLifetime(time.Duration(config.ConnMaxLife) * time.Second)
	if config.ConnMaxIdle > 0 {
		sqlDB.SetConnMaxIdleTime(time.Duration(config.ConnMaxIdle) * time.Second)
	}

	if err = sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	return &GormDrive{
		ID:     uuid.NewString(),
		db:     db,
		Logger: newLogger,
	}, nil
}

// MustNewGorm 是 NewGorm 的 panic 版本，失败时直接 panic，适用于 init 等启动阶段
func MustNewGorm(config *GormConfig) *GormDrive {
	drive, err := NewGorm(config)
	if err != nil {
		panic(err)
	}
	return drive
}
