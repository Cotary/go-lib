package handler

import (
	"bytes"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/log"
	"github.com/Cotary/go-lib/provider/HTTPServer/gin/utils"
	"github.com/gin-gonic/gin"
)

const (
	defaultMaxBodyLogSize = 4 * 1024 // 4KB
)

// defaultHeaderKeys 默认记录的请求头字段，只包含有调试价值且不敏感的字段
var defaultHeaderKeys = []string{"Content-Type", "User-Agent"}

// requestLogConfig 请求日志中间件的配置
type requestLogConfig struct {
	// 请求体日志最大字节数，超出部分截断。0 表示不记录，-1 表示不限制
	maxRequestBodyLog int
	// 响应体日志最大字节数，超出部分截断。0 表示不记录，-1 表示不限制
	maxResponseBodyLog int
	// 是否记录二进制内容的原始数据，默认 false 只记录元信息（Content-Type + 大小）
	logBinaryBody bool
	// 需要记录到日志的请求头字段名列表，nil 使用默认值，空切片表示不记录
	headerKeys []string
	// headerKeysSet 标记用户是否显式设置了 headerKeys，区分 nil（使用默认）和 []string{}（不记录）
	headerKeysSet bool
	// 是否记录所有请求头，为 true 时忽略 headerKeys 配置
	logAllHeaders bool
}

// RequestLogOption 请求日志中间件的选项函数
type RequestLogOption func(*requestLogConfig)

// WithMaxRequestBodyLog 设置请求体日志的最大记录字节数。
// 0 表示不记录请求体，-1 表示不限制大小
func WithMaxRequestBodyLog(size int) RequestLogOption {
	return func(c *requestLogConfig) {
		c.maxRequestBodyLog = size
	}
}

// WithMaxResponseBodyLog 设置响应体日志的最大记录字节数。
// 0 表示不记录响应体，-1 表示不限制大小
func WithMaxResponseBodyLog(size int) RequestLogOption {
	return func(c *requestLogConfig) {
		c.maxResponseBodyLog = size
	}
}

// WithLogBinaryBody 设置是否记录二进制内容的原始数据。
// 默认 false，二进制内容只记录 Content-Type 和大小
func WithLogBinaryBody(enable bool) RequestLogOption {
	return func(c *requestLogConfig) {
		c.logBinaryBody = enable
	}
}

// WithHeaderKeys 设置需要记录到日志的请求头字段名列表。
// 默认记录 Content-Type 和 User-Agent；传入空切片表示不记录任何 header。
// 敏感字段（Authorization、Cookie、X-Token 等）不建议加入列表。
// 如需记录全部 header，请使用 WithLogAllHeaders。
func WithHeaderKeys(keys []string) RequestLogOption {
	return func(c *requestLogConfig) {
		c.headerKeys = keys
		c.headerKeysSet = true
	}
}

// WithLogAllHeaders 设置是否记录所有请求头。
// 开启后忽略 WithHeaderKeys 配置，直接记录完整的 http.Header。
// 注意：完整 header 可能包含 Authorization、Cookie 等敏感信息，
// 仅建议在调试环境中使用。
func WithLogAllHeaders(enable bool) RequestLogOption {
	return func(c *requestLogConfig) {
		c.logAllHeaders = enable
	}
}

// bodyLogWriter 包装 gin.ResponseWriter，在写入客户端的同时将响应体缓冲一份用于日志记录。
// 当缓冲量达到上限后停止缓冲，但不影响向客户端的正常写入。
type bodyLogWriter struct {
	gin.ResponseWriter
	body    *bytes.Buffer
	limit   int // 缓冲上限字节数，-1 表示不限制
	written int // 已缓冲字节数
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	w.bufferBody(b)
	return w.ResponseWriter.Write(b)
}

func (w *bodyLogWriter) WriteString(s string) (int, error) {
	w.bufferBody([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

// bufferBody 将数据写入日志缓冲区，超出上限后自动停止
func (w *bodyLogWriter) bufferBody(data []byte) {
	if w.limit == 0 {
		return
	}
	if w.limit > 0 && w.written >= w.limit {
		return
	}
	toWrite := data
	if w.limit > 0 {
		remaining := w.limit - w.written
		if len(toWrite) > remaining {
			toWrite = toWrite[:remaining]
		}
	}
	w.body.Write(toWrite)
	w.written += len(data)
}

// RequestLogMiddleware 请求/响应日志中间件。
// 记录完整的请求和响应信息，支持通过 Functional Options 配置日志行为：
//   - 大 Body 自动截断，防止日志过大和内存暴涨
//   - 二进制内容（文件上传/下载等）默认只记录元信息，避免乱码
//   - Header 默认只记录安全字段，避免敏感信息泄露
//   - 根据响应状态码自动切换日志级别（5xx Error、4xx Warn、其他 Info）
func RequestLogMiddleware(opts ...RequestLogOption) gin.HandlerFunc {
	cfg := &requestLogConfig{
		maxRequestBodyLog:  defaultMaxBodyLogSize,
		maxResponseBodyLog: defaultMaxBodyLogSize,
		logBinaryBody:      false,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// 确定最终使用的 header key 列表
	logAllHeaders := cfg.logAllHeaders
	headerKeys := defaultHeaderKeys
	if cfg.headerKeysSet {
		headerKeys = cfg.headerKeys
	}

	return func(c *gin.Context) {
		ctx := c.Request.Context()

		requestBody, _ := utils.GetRequestBody(c)
		blw := &bodyLogWriter{
			body:           bytes.NewBuffer(nil),
			ResponseWriter: c.Writer,
			limit:          cfg.maxResponseBodyLog,
		}
		c.Writer = blw
		start := time.Now()

		c.Next()

		end := time.Now()
		status := c.Writer.Status()

		var headerVal interface{}
		if logAllHeaders {
			headerVal = c.Request.Header
		} else {
			headerVal = filterHeaders(c, headerKeys)
		}

		logField := map[string]interface{}{
			"url":                c.Request.URL.String(),
			"start_timestamp":    start.Format(time.DateTime),
			"end_timestamp":      end.Format(time.DateTime),
			"server_name":        c.Request.Host,
			"remote_addr":        c.ClientIP(),
			"proto":              c.Request.Proto,
			"request_method":     c.Request.Method,
			"response_time_ms":   end.Sub(start).Milliseconds(),
			"status":             status,
			"header":             headerVal,
			"request_id":         c.Writer.Header().Get(defined.RequestID),
			"request_body":       formatBodyForLog(requestBody, c.ContentType(), cfg.maxRequestBodyLog, cfg.logBinaryBody),
			"request_body_size":  len(requestBody),
			"response_body":      formatBodyForLog(blw.body.Bytes(), c.Writer.Header().Get("Content-Type"), cfg.maxResponseBodyLog, cfg.logBinaryBody),
			"response_body_size": blw.written,
		}

		logger := log.WithContext(ctx).WithFields(logField)
		switch {
		case status >= 500:
			logger.Error("request log")
		case status >= 400:
			logger.Warn("request log")
		default:
			logger.Info("request log")
		}
	}
}

// filterHeaders 从请求头中提取指定字段，避免记录敏感信息
func filterHeaders(c *gin.Context, keys []string) map[string]string {
	if len(keys) == 0 {
		return nil
	}
	headers := make(map[string]string, len(keys))
	for _, key := range keys {
		if val := c.GetHeader(key); val != "" {
			headers[key] = val
		}
	}
	return headers
}

// isBinaryContentType 判断 Content-Type 是否为二进制类型
func isBinaryContentType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(ct)

	binaryPrefixes := []string{
		"image/",
		"video/",
		"audio/",
		"font/",
	}
	for _, prefix := range binaryPrefixes {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}

	binaryTypes := []string{
		"application/octet-stream",
		"application/zip",
		"application/gzip",
		"application/x-tar",
		"application/x-gzip",
		"application/x-bzip2",
		"application/x-7z-compressed",
		"application/x-rar-compressed",
		"application/pdf",
		"application/vnd.ms-excel",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"multipart/form-data",
	}
	for _, bt := range binaryTypes {
		if ct == bt {
			return true
		}
	}
	return false
}

// formatBodyForLog 根据配置对 Body 内容进行格式化，用于日志记录。
// 处理逻辑：maxSize=0 不记录 → 二进制检测 → 大小截断 → UTF-8 校验
func formatBodyForLog(body []byte, contentType string, maxSize int, logBinary bool) string {
	if maxSize == 0 || len(body) == 0 {
		return ""
	}

	totalSize := len(body)

	if !logBinary && isBinaryContentType(contentType) {
		return fmt.Sprintf("(binary content-type: %s, size: %d bytes)", contentType, totalSize)
	}

	if !logBinary && !utf8.Valid(body) {
		return fmt.Sprintf("(non-utf8 binary, size: %d bytes)", totalSize)
	}

	if maxSize > 0 && totalSize > maxSize {
		truncated := body[:maxSize]
		if !utf8.Valid(truncated) {
			for len(truncated) > 0 && !utf8.Valid(truncated) {
				truncated = truncated[:len(truncated)-1]
			}
		}
		return fmt.Sprintf("%s...(truncated, total: %d bytes)", string(truncated), totalSize)
	}

	return string(body)
}
