package psql

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib"
	"github.com/Cotary/go-lib/common/defined"
	utils2 "github.com/Cotary/go-lib/common/utils"
	"github.com/Cotary/go-lib/log"
	"github.com/Cotary/go-lib/provider/message"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm/logger"
	_ "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
	"time"
)

// RawLogFormatter is a custom logrus.Formatter for outputting raw log messages
type RawLogFormatter struct{}

// Format formats log messages, returning the raw message
func (f *RawLogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	return []byte(entry.Message + "\n\n"), nil
}

// NewGormLogger creates a custom GORM logger
func NewGormLogger(log log.Logger) *GormLogWriter {
	return &GormLogWriter{log}
}

// GormLogWriter is a custom GORM logger
type GormLogWriter struct {
	Log log.Logger
}

// Printf implements the GORM logger interface
func (l *GormLogWriter) Printf(format string, v ...any) {
	l.Log.Info(fmt.Sprintf(format, v...))
}

type GormLogger struct {
	logger.Writer
	logger.Config
	message.Sender
	infoStr, warnStr, errStr            string
	traceStr, traceErrStr, traceWarnStr string
}

func (l *GormLogger) SetSender(s message.Sender) {
	l.Sender = s
}

// New creates a new GORM logger with custom configurations
func New(writer logger.Writer, config logger.Config) *GormLogger {
	var (
		infoStr      = "%s %s [%s] [info] "
		warnStr      = "%s %s [%s] [warn] "
		errStr       = "%s %s [%s] [error] "
		traceStr     = "%s %s [%s] [%.3fms] [rows:%v] %s"
		traceWarnStr = "%s %s [%s] %s [%.3fms] [rows:%v] %s"
		traceErrStr  = "%s %s [%s] %s [%.3fms] [rows:%v] %s"
	)

	if config.Colorful {
		infoStr = logger.Green + "%s %s [%s] " + logger.Reset + logger.Green + "[info] " + logger.Reset
		warnStr = logger.BlueBold + "%s %s [%s] " + logger.Reset + logger.Magenta + "[warn] " + logger.Reset
		errStr = logger.Magenta + "%s %s [%s] " + logger.Reset + logger.Red + "[error] " + logger.Reset
		traceStr = logger.Green + "%s %s [%s] " + logger.Reset + logger.Yellow + "[%.3fms] " + logger.BlueBold + "[rows:%v]" + logger.Reset + " %s"
		traceWarnStr = logger.Green + "%s %s [%s] " + logger.Yellow + "%s " + logger.Reset + logger.RedBold + "[%.3fms] " + logger.Yellow + "[rows:%v]" + logger.Magenta + " %s" + logger.Reset
		traceErrStr = logger.RedBold + "%s %s [%s] " + logger.MagentaBold + "%s " + logger.Reset + logger.Yellow + "[%.3fms] " + logger.BlueBold + "[rows:%v]" + logger.Reset + " %s"
	}

	return &GormLogger{
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

// LogMode sets the log mode
func (l *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newlogger := *l
	newlogger.LogLevel = level
	return &newlogger
}

// getRequestInfo retrieves the request ID from the context
func getRequestInfo(ctx context.Context) string {
	requestID, _ := ctx.Value(defined.RequestID).(string)
	requestUri, _ := ctx.Value(defined.RequestURI).(string)
	str := fmt.Sprintf("requestID: %s; requestUri: %s", requestID, requestUri)
	return str
}

// Info prints info messages
func (l GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Info {
		l.Printf(l.infoStr+msg, append([]interface{}{time.Now().Format(time.DateTime), utils.FileWithLineNum(), getRequestInfo(ctx)}, data...)...)
	}
}

// Warn prints warning messages
func (l GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Warn {
		l.Printf(l.warnStr+msg, append([]interface{}{time.Now().Format(time.DateTime), utils.FileWithLineNum(), getRequestInfo(ctx)}, data...)...)
	}
}

// Error prints error messages
func (l GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Error {
		l.Printf(l.errStr+msg, append([]interface{}{time.Now().Format(time.DateTime), utils.FileWithLineNum(), getRequestInfo(ctx)}, data...)...)
	}
}

// Trace prints SQL trace messages
func (l GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && l.LogLevel >= logger.Error && (!errors.Is(err, logger.ErrRecordNotFound) || !l.IgnoreRecordNotFoundError):
		sql, rows := fc()
		if rows == -1 {
			l.Printf(l.traceErrStr, time.Now().Format(time.DateTime), utils.FileWithLineNum(), getRequestInfo(ctx), err, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			l.Printf(l.traceErrStr, time.Now().Format(time.DateTime), utils.FileWithLineNum(), getRequestInfo(ctx), err, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	case elapsed > l.SlowThreshold && l.SlowThreshold != 0 && l.LogLevel >= logger.Warn:
		sql, rows := fc()
		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.SlowThreshold)
		if rows == -1 {
			l.Printf(l.traceWarnStr, time.Now().Format(time.DateTime), utils.FileWithLineNum(), getRequestInfo(ctx), slowLog, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			l.Printf(l.traceWarnStr, time.Now().Format(time.DateTime), utils.FileWithLineNum(), getRequestInfo(ctx), slowLog, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}

		msgStr := fmt.Sprintf(l.traceWarnStr, time.Now().Format(time.DateTime), utils.FileWithLineNum(), getRequestInfo(ctx), slowLog, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		SendMessage(ctx, l.Sender, msgStr)
	case l.LogLevel == logger.Info:
		sql, rows := fc()
		if rows == -1 {
			l.Printf(l.traceStr, time.Now().Format(time.DateTime), utils.FileWithLineNum(), getRequestInfo(ctx), float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			l.Printf(l.traceStr, time.Now().Format(time.DateTime), utils.FileWithLineNum(), getRequestInfo(ctx), float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	}
}

func SendMessage(ctx context.Context, sender message.Sender, msg string) {
	sender = message.GetPrioritySender(sender)
	if sender == nil {
		return
	}
	env := lib.Env
	serverName := lib.ServerName
	requestID, _ := ctx.Value(defined.RequestID).(string)
	requestUri, _ := ctx.Value(defined.RequestURI).(string)
	requestJson, _ := ctx.Value(defined.RequestBodyJson).(string)

	zMap := utils2.NewZMap[string, string]().
		Set("ServerName:", serverName).
		Set("Env:", env).
		Set("RequestID:", requestID).
		Set("RequestUri:", requestUri).
		Set("RequestJson:", requestJson).
		Set("SqlInfo:", msg)

	sender.Send(ctx, "Slow Query", zMap)
}
