package psql

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm/logger"
	_ "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
	"time"
)

// NewGormLogger 创建一个自定义的 GORM 日志记录器
func NewGormLogger(log *logrus.Logger) *GormLogger {
	return &GormLogger{log}
}

// GormLogger 是一个自定义的 GORM 日志记录器
type GormLogger struct {
	Log *logrus.Logger
}

// Print 实现 GORM 的日志记录接口
func (l *GormLogger) Printf(format string, v ...any) {
	l.Log.Printf(format, v...)

}

// RawLogFormatter 是自定义的 logrus.Formatter，用于输出原始日志消息
type RawLogFormatter struct{}

// Format 格式化日志消息，这里直接返回原始消息
func (f *RawLogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	nowTime := time.Now().Format(time.DateTime)
	return []byte(nowTime + "  " + entry.Message + "\n\n"), nil
}

func New(writer logger.Writer, config logger.Config) logger.Interface {
	var (
		infoStr      = "%s [%s]\n[info] "
		warnStr      = "%s [%s]\n[warn] "
		errStr       = "%s [%s]\n[error] "
		traceStr     = "%s [%s]\n[%.3fms] [rows:%v] %s"
		traceWarnStr = "%s [%s] %s\n[%.3fms] [rows:%v] %s"
		traceErrStr  = "%s [%s] %s\n[%.3fms] [rows:%v] %s"
	)

	if config.Colorful {
		infoStr = logger.Green + "%s [%s]\n" + logger.Reset + logger.Green + "[info] " + logger.Reset
		warnStr = logger.BlueBold + "%s [%s]\n" + logger.Reset + logger.Magenta + "[warn] " + logger.Reset
		errStr = logger.Magenta + "%s [%s]\n" + logger.Reset + logger.Red + "[error] " + logger.Reset
		traceStr = logger.Green + "%s [%s]\n" + logger.Reset + logger.Yellow + "[%.3fms] " + logger.BlueBold + "[rows:%v]" + logger.Reset + " %s"
		traceWarnStr = logger.Green + "%s [%s] " + logger.Yellow + "%s\n" + logger.Reset + logger.RedBold + "[%.3fms] " + logger.Yellow + "[rows:%v]" + logger.Magenta + " %s" + logger.Reset
		traceErrStr = logger.RedBold + "%s [%s] " + logger.MagentaBold + "%s\n" + logger.Reset + logger.Yellow + "[%.3fms] " + logger.BlueBold + "[rows:%v]" + logger.Reset + " %s"
	}

	return &gormLogger{
		Writer:       writer,
		Config:       config,
		infoStr:      infoStr,
		warnStr:      warnStr,
		errStr:       errStr,
		traceStr:     traceStr,
		traceWarnStr: traceWarnStr,
		traceErrStr:  traceErrStr,
	}
}

type gormLogger struct {
	logger.Writer
	logger.Config
	infoStr, warnStr, errStr            string
	traceStr, traceErrStr, traceWarnStr string
}

// LogMode log mode
func (l *gormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newlogger := *l
	newlogger.LogLevel = level
	return &newlogger
}
func getRequestID(ctx context.Context) string {
	//requestID Time
	reqV := ctx.Value(defined.RequestID)
	reqID, _ := reqV.(string)
	//formatTime := time.Now().Format(defined.TimeLayout)
	str := "requestID:" + reqID
	return str
}

// Info print info
func (l gormLogger) Info(ctx context.Context, msg string, data ...interface{}) {

	if l.LogLevel >= logger.Info {
		l.Printf(l.infoStr+msg, append([]interface{}{utils.FileWithLineNum(), getRequestID(ctx)}, data...)...)
	}
}

// Warn print warn messages
func (l gormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Warn {
		l.Printf(l.warnStr+msg, append([]interface{}{utils.FileWithLineNum(), getRequestID(ctx)}, data...)...)
	}
}

// Error print error messages
func (l gormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Error {
		l.Printf(l.errStr+msg, append([]interface{}{utils.FileWithLineNum(), getRequestID(ctx)}, data...)...)
	}
}

// Trace print sql message
func (l gormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && l.LogLevel >= logger.Error && (!errors.Is(err, logger.ErrRecordNotFound) || !l.IgnoreRecordNotFoundError):
		sql, rows := fc()
		if rows == -1 {
			l.Printf(l.traceErrStr, utils.FileWithLineNum(), getRequestID(ctx), err, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			l.Printf(l.traceErrStr, utils.FileWithLineNum(), getRequestID(ctx), err, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	case elapsed > l.SlowThreshold && l.SlowThreshold != 0 && l.LogLevel >= logger.Warn:
		sql, rows := fc()
		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.SlowThreshold)
		if rows == -1 {
			l.Printf(l.traceWarnStr, utils.FileWithLineNum(), getRequestID(ctx), slowLog, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			l.Printf(l.traceWarnStr, utils.FileWithLineNum(), getRequestID(ctx), slowLog, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	case l.LogLevel == logger.Info:
		sql, rows := fc()
		if rows == -1 {
			l.Printf(l.traceStr, utils.FileWithLineNum(), getRequestID(ctx), float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			l.Printf(l.traceStr, utils.FileWithLineNum(), getRequestID(ctx), float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	}
}
