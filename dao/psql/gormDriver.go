package psql

import (
	"github.com/google/uuid"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"time"
)

type GormConfig struct {
	Driver      string   `yaml:"driver"`
	Dsn         []string `yaml:"dsn"`
	IdleTimeout int      `yaml:"idleTimeout"` //设置连接的最大生命周期,超过这个时间的连接将被关闭并重新建立。
	MaxOpens    int      `yaml:"maxOpens"`    //设置数据库的最大打开连接数。
	MaxIdles    int      `yaml:"maxIdles"`    //设置连接池中保持空闲状态的最大连接数。

	LogDir        string `yaml:"log_dir"`
	LogLevel      string `yaml:"log_level"`
	SlowThreshold int    `yaml:"slow_threshold"` // 慢sql阈值 ms
	LogSaveDay    int    `yaml:"log_save_day"`   //日志保留天数
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

func handleConfig(config *GormConfig) {
	if config.LogDir == "" {
		config.LogDir = "./logs/gorm"
	}
	if config.LogLevel == "" {
		config.LogLevel = "info"
	}
	if config.LogSaveDay == 0 {
		config.LogSaveDay = 30
	}
	if config.SlowThreshold == 0 {
		config.SlowThreshold = 500
	}

	if config.IdleTimeout == 0 {
		config.IdleTimeout = 30
	}
	if config.MaxOpens == 0 {
		config.MaxOpens = 50
	}
	if config.MaxIdles == 0 {
		config.MaxIdles = 15
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

func NewGorm(c GormConfig) *GormDrive {

	handleConfig(&c)
	// 创建日志目录
	writer, err := rotatelogs.New(
		c.LogDir+"/%Y%m%d%H.log",
		rotatelogs.WithMaxAge(time.Duration(c.LogSaveDay)*24*time.Hour), // 保留 x 天的日志
		rotatelogs.WithRotationTime(time.Hour),                          // 每小时分割一次日志
	)
	if err != nil {
		panic("gorm create log dir error:" + err.Error())
	}
	log := logrus.New()
	log.SetOutput(writer)
	log.SetFormatter(&RawLogFormatter{})

	newLogger := New(
		NewGormLogger(log), // io writer
		logger.Config{
			SlowThreshold:             time.Duration(c.SlowThreshold) * time.Millisecond, // Slow SQL threshold
			LogLevel:                  getLogLevelEnum(c.LogLevel),                       // Log level
			IgnoreRecordNotFoundError: false,                                             // Ignore ErrRecordNotFound error for logger
			ParameterizedQueries:      false,
			Colorful:                  false,
		},
	)
	driver := getDriver(c.Driver, c.Dsn)
	if driver == nil {
		panic("driver not support:" + c.Driver)
	}

	db, err := gorm.Open(driver, &gorm.Config{
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

	if err = sqlDB.Ping(); err != nil {
		panic(err)
	}
	return NewGormDrive(db)
}
