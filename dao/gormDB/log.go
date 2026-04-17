package gormDB

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Cotary/go-lib/common/appctx"
	"github.com/Cotary/go-lib/common/defined"
	utils2 "github.com/Cotary/go-lib/common/utils"
	"github.com/Cotary/go-lib/log"
	"github.com/Cotary/go-lib/provider/message"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

// logLineEnding 根据运行时操作系统确定日志换行符。
// Windows 使用 \r\n 以兼容部分只认 CRLF 的日志查看工具，其他系统使用 \n。
var logLineEnding = func() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}()

// GormLogWriter 将 go-lib/log 适配为 GORM 的 logger.Writer 接口
type GormLogWriter struct {
	Log log.Logger
}

func NewGormLogWriter(l log.Logger) *GormLogWriter {
	return &GormLogWriter{Log: l}
}

// Printf 追加平台相关换行符后调用底层日志输出
func (l *GormLogWriter) Printf(format string, v ...any) {
	l.Log.Raw(fmt.Sprintf(format+logLineEnding, v...))
}

// GormLogger 是 GORM 的自定义日志记录器
//   - 支持 message.Sender 将慢 SQL 告警推送到外部
//   - 从 context 提取 TransactionID / RequestID / RequestURI 等追踪信息注入日志
type GormLogger struct {
	logger.Writer
	logger.Config
	sender                                  message.Sender
	infoStr, warnStr, errStr                string
	traceInfoStr, traceErrStr, traceWarnStr string
}

// SetSender 设置消息发送器，为 nil 时不推送慢 SQL 告警
func (l *GormLogger) SetSender(s message.Sender) {
	l.sender = s
}

func NewGormLogger(writer logger.Writer, config logger.Config) *GormLogger {
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

		traceInfoStr = logger.Cyan + "[info] " + logger.Reset + logger.Green + "%s %s [%s] \n" + logger.Reset +
			logger.Yellow + "[%.3fms] " + logger.BlueBold + "[rows:%v]" + logger.Reset + " %s"
		traceWarnStr = logger.RedBold + "[warn] " + logger.Reset + logger.Green + "%s %s [%s] " +
			logger.Yellow + "%s \n" + logger.Reset + logger.RedBold + "[%.3fms] " +
			logger.Yellow + "[rows:%v]" + logger.Magenta + " %s" + logger.Reset
		traceErrStr = logger.RedBold + "[error] " + logger.Reset + logger.RedBold + "%s %s [%s] " +
			logger.MagentaBold + "%s \n" + logger.Reset + logger.Yellow + "[%.3fms] " +
			logger.BlueBold + "[rows:%v]" + logger.Reset + " %s"
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

// LogMode 复制 receiver 并返回新实例，对并发 goroutine 安全（GORM 要求此行为）
func (l *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newlogger := *l
	newlogger.LogLevel = level
	return &newlogger
}

// getRequestInfo 从 context 提取请求追踪 ID，用于将业务请求与 SQL 日志关联。
// 提取顺序：TransactionID -> RequestID -> RequestURI。
func getRequestInfo(ctx context.Context) string {
	txID, _ := ctx.Value(defined.TransactionID).(string)
	requestID, _ := ctx.Value(defined.RequestID).(string)
	requestUri, _ := ctx.Value(defined.RequestURI).(string)

	var parts []string
	if txID != "" {
		parts = append(parts, "txID: "+txID)
	}
	if requestID != "" {
		parts = append(parts, "requestID: "+requestID)
	}
	if requestUri != "" {
		parts = append(parts, "requestUri: "+requestUri)
	}
	return strings.Join(parts, "; ")
}

func (l GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Info {
		l.Printf(l.infoStr+msg, append([]interface{}{time.Now().Format(time.DateTime), utils.FileWithLineNum(), getRequestInfo(ctx)}, data...)...)
	}
}

func (l GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Warn {
		l.Printf(l.warnStr+msg, append([]interface{}{time.Now().Format(time.DateTime), utils.FileWithLineNum(), getRequestInfo(ctx)}, data...)...)
	}
}

func (l GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Error {
		l.Printf(l.errStr+msg, append([]interface{}{time.Now().Format(time.DateTime), utils.FileWithLineNum(), getRequestInfo(ctx)}, data...)...)
	}
}

// Trace 是 GORM 执行 SQL 后的回调入口
//   - 发生错误时以 error 级别记录，ErrRecordNotFound 可按配置忽略
//   - 超过 SlowThreshold 时以 warn 级别记录并通过 Sender 推送告警
//   - 正常执行以 info 级别记录
func (l GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	elapsedMs := float64(elapsed.Nanoseconds()) / 1e6
	fileLine := utils.FileWithLineNum()
	reqInfo := getRequestInfo(ctx)
	currTime := time.Now().Format(time.DateTime)

	sql, rowsCount := fc()
	rows := "-"
	if rowsCount != -1 {
		rows = strconv.FormatInt(rowsCount, 10)
	}

	switch {
	case err != nil && l.LogLevel >= logger.Error && (!errors.Is(err, logger.ErrRecordNotFound) || !l.IgnoreRecordNotFoundError):
		l.Printf(l.traceErrStr, currTime, fileLine, reqInfo, err, elapsedMs, rows, sql)

	case elapsed > l.SlowThreshold && l.SlowThreshold != 0 && l.LogLevel >= logger.Warn:
		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.SlowThreshold)
		msg := fmt.Sprintf(l.traceWarnStr, currTime, fileLine, reqInfo, slowLog, elapsedMs, rows, sql)
		l.Printf("%s", msg)
		sendMessage(ctx, l.sender, msg)

	case l.LogLevel == logger.Info:
		l.Printf(l.traceInfoStr, currTime, fileLine, reqInfo, elapsedMs, rows, sql)
	}
}

// sendMessage 将慢 SQL 告警通过 Sender 推送到外部，若 Sender 为 nil 则回退到全局 Sender。
// 消息中包含 gormDB 运行环境和请求上下文等排查信息。
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
