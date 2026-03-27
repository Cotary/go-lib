package gormDB

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Cotary/go-lib/common/appctx"
	"github.com/Cotary/go-lib/common/defined"
	utils2 "github.com/Cotary/go-lib/common/utils"
	"github.com/Cotary/go-lib/log"
	"github.com/Cotary/go-lib/provider/message"
	"github.com/pkg/errors"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

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
	l.Log.Raw(fmt.Sprintf(format+"\r\n", v...))
}

type GormLogger struct {
	logger.Writer
	logger.Config
	message.Sender
	infoStr, warnStr, errStr                string
	traceInfoStr, traceErrStr, traceWarnStr string
}

func (l *GormLogger) SetSender(s message.Sender) {
	l.Sender = s
}

// New creates a new GORM logger with custom configurations
func New(writer logger.Writer, config logger.Config) *GormLogger {
	var (
		infoStr      = "[info] %s %s [%s] \n"
		warnStr      = "[warn] %s %s [%s] \n"
		errStr       = "[error] %s %s [%s] \n"
		traceInfoStr = "[info] %s %s [%s] \n[%.3fms] [rows:%v] %s"
		traceWarnStr = "[warn] %s %s [%s] %s \n[%.3fms] [rows:%v] %s"
		traceErrStr  = "[error] %s %s [%s] %s \n[%.3fms] [rows:%v] %s"
	)

	if config.Colorful {
		infoStr = logger.Green + "[info] " + logger.Reset + logger.Green + "%s %s [%s] \n" + logger.Reset
		warnStr = logger.Magenta + "[warn] " + logger.Reset + logger.BlueBold + "%s %s [%s] \n" + logger.Reset
		errStr = logger.Red + "[error] " + logger.Reset + logger.Magenta + "%s %s [%s] \n" + logger.Reset

		// Trace 相关（SQL 日志）
		traceInfoStr = logger.Cyan + "[info] " + logger.Reset + logger.Green + "%s %s [%s] \n" + logger.Reset + logger.Yellow + "[%.3fms] " + logger.BlueBold + "[rows:%v]" + logger.Reset + " %s"

		traceWarnStr = logger.RedBold + "[warn] " + logger.Reset + logger.Green + "%s %s [%s] " + logger.Yellow + "%s \n" + logger.Reset + logger.RedBold + "[%.3fms] " + logger.Yellow + "[rows:%v]" + logger.Magenta + " %s" + logger.Reset

		traceErrStr = logger.RedBold + "[error] " + logger.Reset + logger.RedBold + "%s %s [%s] " + logger.MagentaBold + "%s \n" + logger.Reset + logger.Yellow + "[%.3fms] " + logger.BlueBold + "[rows:%v]" + logger.Reset + " %s"
	}

	return &GormLogger{
		Writer:       writer,
		Config:       config,
		infoStr:      infoStr,
		warnStr:      warnStr,
		errStr:       errStr,
		traceInfoStr: traceInfoStr,
		traceWarnStr: traceWarnStr,
		traceErrStr:  traceErrStr,
	}
}

// LogMode sets the logger mode
func (l *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newlogger := *l
	newlogger.LogLevel = level
	return &newlogger
}

// getRequestInfo retrieves the request ID and transaction ID from the context
func getRequestInfo(ctx context.Context) string {
	// 1. 提取变量
	txID, _ := ctx.Value(defined.TransactionID).(string)
	requestID, _ := ctx.Value(defined.RequestID).(string)
	requestUri, _ := ctx.Value(defined.RequestURI).(string)

	// 2. 使用切片收集非空字段
	var parts []string

	// 优先放置 TransactionID (txID)
	if txID != "" {
		parts = append(parts, "txID: "+txID)
	}
	if requestID != "" {
		parts = append(parts, "requestID: "+requestID)
	}
	if requestUri != "" {
		parts = append(parts, "requestUri: "+requestUri)
	}

	// 3. 使用分号安全拼接
	return strings.Join(parts, "; ")
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

func (l GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	elapsedMs := float64(elapsed.Nanoseconds()) / 1e6
	fileLine := utils.FileWithLineNum()
	reqInfo := getRequestInfo(ctx)
	currTime := time.Now().Format(time.DateTime)

	// 1. 统一获取 SQL 和行数，并格式化 rows
	sql, rowsCount := fc()
	rows := "-"
	if rowsCount != -1 {
		rows = strconv.FormatInt(rowsCount, 10)
	}

	// 2. 使用 switch 处理不同的日志级别逻辑
	switch {
	case err != nil && l.LogLevel >= logger.Error && (!errors.Is(err, logger.ErrRecordNotFound) || !l.IgnoreRecordNotFoundError):
		// 错误日志
		l.Printf(l.traceErrStr, currTime, fileLine, reqInfo, err, elapsedMs, rows, sql)

	case elapsed > l.SlowThreshold && l.SlowThreshold != 0 && l.LogLevel >= logger.Warn:
		// 慢查询日志
		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.SlowThreshold)
		msg := fmt.Sprintf(l.traceWarnStr, currTime, fileLine, reqInfo, slowLog, elapsedMs, rows, sql)
		l.Printf(msg)
		sendMessage(ctx, l.Sender, msg)

	case l.LogLevel == logger.Info:
		// 常规查询日志
		l.Printf(l.traceInfoStr, currTime, fileLine, reqInfo, elapsedMs, rows, sql)
	}
}

func sendMessage(ctx context.Context, sender message.Sender, msg string) {
	if sender == nil {
		sender = message.GetGlobalSender()
	}
	if sender == nil {
		return
	}
	env := appctx.Env()
	serverName := appctx.ServerName()
	requestID, _ := ctx.Value(defined.RequestID).(string)
	requestUri, _ := ctx.Value(defined.RequestURI).(string)
	requestJson, _ := ctx.Value(defined.RequestBodyJson).(string)

	zMap := utils2.NewOrderedMap[string, string]().
		Set("ServerName:", serverName).
		Set("Env:", env).
		Set("RequestID:", requestID).
		Set("RequestUri:", requestUri).
		Set("RequestJson:", requestJson).
		Set("SqlInfo:", msg)

	sender.Send(ctx, "Slow Query", zMap)
}
