package psql

import (
	"github.com/google/uuid"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"time"
)

type GormConfig struct {
	Driver      string   `yaml:"driver"`
	Dsn         []string `yaml:"dsn"`
	IdleTimeout int64    `yaml:"idleTimeout"`
	MaxOpens    int      `yaml:"maxOpens"`
	MaxIdles    int      `yaml:"maxIdles"`
	Debug       bool     `yaml:"debug"`
	LogDir      string   `yaml:"log_dir"`
}

type GormDrive struct {
	id string
	*gorm.DB
}

func NewGormDrive(db *gorm.DB) *GormDrive {
	return &GormDrive{
		id: uuid.NewString(),
		DB: db,
	}
}

func NewGorm(c *GormConfig) *GormDrive {

	// 创建日志目录
	logPath := "./logs/gorm"
	if c.LogDir != "" {
		logPath = c.LogDir
	}
	writer, _ := rotatelogs.New(
		logPath+"/%Y%m%d%H.log",
		rotatelogs.WithMaxAge(10*24*time.Hour), // 保留 7 天的日志
		rotatelogs.WithRotationTime(time.Hour), // 每小时分割一次日志
	)
	log := logrus.New()
	log.SetOutput(writer)
	log.SetFormatter(&RawLogFormatter{})
	newLogger := New(
		NewGormLogger(log), // io writer
		logger.Config{
			SlowThreshold:             time.Second,   // Slow SQL threshold
			LogLevel:                  logger.Silent, // Log level
			IgnoreRecordNotFoundError: false,         // Ignore ErrRecordNotFound error for logger
			ParameterizedQueries:      false,
			Colorful:                  false,
		},
	)
	db, err := gorm.Open(postgres.Open(c.Dsn[0]), &gorm.Config{
		SkipDefaultTransaction: true, //禁用默认事务
		PrepareStmt:            true, //缓存预编译语句
		Logger:                 newLogger,
	})
	if err != nil {
		panic(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic(err)
	}
	sqlDB.SetMaxIdleConns(c.MaxIdles)
	sqlDB.SetMaxOpenConns(c.MaxOpens)
	sqlDB.SetConnMaxLifetime(time.Duration(c.IdleTimeout))
	if c.Debug {
		db = db.Debug()
	}
	if err = sqlDB.Ping(); err != nil {
		panic(err)
	}

	gd := NewGormDrive(db)
	return gd
}
