package gormDB

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Cotary/go-lib/common/appctx"
	"github.com/Cotary/go-lib/common/defined"
	utils2 "github.com/Cotary/go-lib/common/utils"
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

// GormLogger 是 GORM 的自定义日志记录器。
//
//   - 直接持有 io.Writer，自带 Printf，不再需要 GormLogWriter 适配层；
//   - 慢 SQL 通过 message.Default() 推送告警；
//   - 从 context 提取 TransactionID / RequestID / RequestURI 注入日志。
//
// 格式化模板在构造时预生成，避免每条日志重复拼接。
type GormLogger struct {
	logger.Config
	out io.Writer

	infoStr, warnStr, errStr                string
	traceInfoStr, traceErrStr, traceWarnStr string
}

var _ logger.Interface = (*GormLogger)(nil)

// NewGormLogger 构造 GORM logger：out 决定日志写到哪里，config 控制级别/阈值。
func NewGormLogger(out io.Writer, config logger.Config) *GormLogger {
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
		Config:       config,
		out:          out,
		infoStr:      infoStr,
		warnStr:      warnStr,
		errStr:       errStr,
		traceInfoStr: traceInfoStr,
		traceWarnStr: traceWarnStr,
		traceErrStr:  traceErrStr,
	}
}

// Printf 把格式化后的文本写入 out，去掉结尾换行后统一补一个，
// 避免出现空行或 Windows 下的 \n\r\n。
func (l GormLogger) Printf(format string, v ...any) {
	line := strings.TrimRight(fmt.Sprintf(format, v...), "\r\n")
	_, _ = io.WriteString(l.out, line+logLineEnding)
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
		sendMessage(ctx, msg)

	case l.LogLevel == logger.Info:
		l.Printf(l.traceInfoStr, currTime, fileLine, reqInfo, elapsedMs, rows, sql)
	}
}

// sendMessage 将慢 SQL 告警通过进程默认出口推送（message.Default 永远非 nil）。
// 消息中包含 gormDB 运行环境和请求上下文等排查信息。
func sendMessage(ctx context.Context, msg string) {
	id := appctx.OrDefault(ctx)
	env := id.Env
	serverName := id.Service
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

	_ = message.Default().Send(ctx, "Slow Query", zMap)
}
