package pgsql

import (
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

type GormConfig struct {
	Driver      string   `mapstructure:"driver" yaml:"driver"`
	Dsn         []string `mapstructure:"dsn" yaml:"dsn"`
	ConnMaxLife int      `mapstructure:"connMaxLife" yaml:"connMaxLife"` // 秒,连接最大生命周期,超过后将被关闭并重建。0 表示不限制。
	ConnMaxIdle int      `mapstructure:"connMaxIdle" yaml:"connMaxIdle"` // 秒,空闲连接最大存活时间。0 表示不限制。
	MaxOpens    int      `mapstructure:"maxOpens" yaml:"maxOpens"`       // 最大打开连接数。0 表示不限制。
	MaxIdles    int      `mapstructure:"maxIdles" yaml:"maxIdles"`       // 连接池最大空闲连接数。默认 2。
	IdleTimeout int      `mapstructure:"idleTimeout" yaml:"idleTimeout"` // Deprecated: 请使用 ConnMaxLife

	LogDir        string `mapstructure:"log_dir" yaml:"log_dir"`
	LogLevel      string `mapstructure:"log_level" yaml:"log_level"`           // 日志等级 silent error warn info
	SlowThreshold int64  `mapstructure:"slow_threshold" yaml:"slow_threshold"` // 慢 SQL 阈值(ms)
	LogSaveDay    int64  `mapstructure:"log_save_day" yaml:"log_save_day"`     // 日志保留天数
}

type GormDrive struct {
	ID     string
	Logger *GormLogger
	db     *gorm.DB
}

func (g *GormDrive) DB() *gorm.DB {
	return g.db
}

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
	// 兼容旧字段
	if config.ConnMaxLife == 0 && config.IdleTimeout > 0 {
		config.ConnMaxLife = config.IdleTimeout
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
	newLogger := New(
		NewGormLogger(writer),
		logger.Config{
			SlowThreshold:             time.Duration(config.SlowThreshold) * time.Millisecond,
			LogLevel:                  getLogLevelEnum(config.LogLevel),
			IgnoreRecordNotFoundError: false,
			ParameterizedQueries:      false,
			Colorful:                  false,
		},
	)
	driver := getDriver(config.Driver, config.Dsn)
	if driver == nil {
		return nil, fmt.Errorf("unsupported database driver: %s", config.Driver)
	}

	db, err := gorm.Open(driver, &gorm.Config{
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

// MustNewGorm 与 NewGorm 相同，但遇到错误时 panic。适合在 init 阶段使用。
func MustNewGorm(config *GormConfig) *GormDrive {
	drive, err := NewGorm(config)
	if err != nil {
		panic(err)
	}
	return drive
}
