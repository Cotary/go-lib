package http

import (
	"fmt"
	"net/url"
	"time"

	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/log"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
)

// ============================================================================
// 内置日志中间件
// ============================================================================

// RequestLogMiddleware 请求日志中间件（内置）
//
// 该中间件会自动从上下文中读取日志配置，记录完整的请求/响应信息。
// 由 RequestBuilder 自动添加，通常不需要手动使用。
func RequestLogMiddleware() Middleware {
	return func(ctx *Context) {
		ctx.Next()

		cfg := getLogConfig(ctx)
		if cfg == nil || !cfg.keepLog {
			return
		}

		logMap := ctx.BuildLogMap()
		logger := log.WithContext(ctx.Ctx)

		if ctx.Error != nil {
			if logger != nil {
				logger.WithContext(ctx.Ctx).WithFields(logMap).Error("HTTP Request")
			}
			if cfg.sendErrorMsg {
				e.SendMessage(ctx.Ctx, errors.New("HTTP Request Error:"+utils.Json(logMap)))
			}
		} else {
			if logger != nil {
				logger.WithContext(ctx.Ctx).WithFields(logMap).Info("HTTP Request")
			}
		}
	}
}

// getLogConfig 从上下文获取日志配置
func getLogConfig(ctx *Context) *logConfig {
	if v, ok := ctx.Get(logConfigKey); ok {
		if cfg, ok := v.(*logConfig); ok {
			return cfg
		}
	}
	return nil
}

// ============================================================================
// 通用中间件（前置 + 后置）
// ============================================================================

// RecoveryMiddleware 恢复中间件，捕获 panic 防止程序崩溃
//
// 建议放在中间件链的最外层
func RecoveryMiddleware() Middleware {
	return func(ctx *Context) {
		defer func() {
			if r := recover(); r != nil {
				ctx.AddError(fmt.Errorf("panic recovered: %v", r))
			}
		}()
		ctx.Next()
	}
}

// TimingMiddleware 计时中间件，记录请求耗时
//
// 耗时会存储在 ctx.Get("request_duration") 中
func TimingMiddleware() Middleware {
	return func(ctx *Context) {
		startTime := time.Now()
		ctx.Next()
		duration := time.Since(startTime)

		ctx.Set("request_duration", duration)
		if ctx.Response != nil && ctx.Response.Stats != nil {
			ctx.Response.Stats.TotalTime = duration
			ctx.Response.Stats.EndTime = time.Now()
		}
	}
}

// LoggingMiddleware 日志中间件，使用自定义 Logger 记录请求和响应信息
func LoggingMiddleware(logger log.Logger) Middleware {
	return func(ctx *Context) {
		startTime := time.Now()
		requestID := ctx.Ctx.Value(defined.RequestID)

		if logger != nil {
			logger.WithContext(ctx.Ctx).WithFields(map[string]interface{}{
				"request_id": requestID,
				"method":     ctx.Request.Method,
				"url":        ctx.Request.URL,
				"phase":      "request_start",
			}).Debug("HTTP Request Starting")
		}

		ctx.Next()

		duration := time.Since(startTime)
		logFields := map[string]interface{}{
			"request_id": requestID,
			"method":     ctx.Request.Method,
			"url":        ctx.Request.URL,
			"duration":   duration.String(),
			"phase":      "request_complete",
		}

		if ctx.Response != nil {
			logFields["status_code"] = ctx.Response.StatusCode
			logFields["response_size"] = len(ctx.Response.Body)
		}

		if ctx.Error != nil {
			logFields["error"] = ctx.Error.Error()
			if logger != nil {
				logger.WithContext(ctx.Ctx).WithFields(logFields).Error("HTTP Request Failed")
			}
		} else {
			if logger != nil {
				logger.WithContext(ctx.Ctx).WithFields(logFields).Info("HTTP Request Completed")
			}
		}
	}
}

// TracingMiddleware 追踪中间件，添加追踪 ID
func TracingMiddleware() Middleware {
	return func(ctx *Context) {
		requestID := ctx.Ctx.Value(defined.RequestID)
		if requestID == nil {
			requestID = uuid.NewString()
		}

		if ctx.Request.Headers == nil {
			ctx.Request.Headers = make(map[string]string)
		}
		ctx.Request.Headers["X-Request-ID"] = fmt.Sprintf("%v", requestID)

		ctx.Next()

		ctx.Set("trace_id", requestID)
	}
}

// RetryMiddleware 重试中间件，自动重试失败的请求
//
// 重试条件：存在传输错误（ctx.Error != nil）或响应状态码为 5xx。
// 4xx 错误不会重试。重试时只重新发起 HTTP 请求，不重新执行中间件链。
func RetryMiddleware(maxRetries int, retryDelay time.Duration) Middleware {
	return func(ctx *Context) {
		ctx.Next()

		for attempt := 0; attempt < maxRetries; attempt++ {
			needRetry := ctx.Error != nil
			if !needRetry && ctx.Response != nil && ctx.Response.StatusCode >= 500 {
				needRetry = true
			}
			if !needRetry {
				return
			}

			// 4xx 错误不重试
			if ctx.Response != nil && ctx.Response.StatusCode >= 400 && ctx.Response.StatusCode < 500 {
				return
			}

			time.Sleep(retryDelay)
			ctx.RetryRequest()
		}
	}
}

// ============================================================================
// 请求前置中间件
// ============================================================================

// AuthMiddleware 应用认证中间件，添加签名认证头
func AuthMiddleware(appID, secret, signType string) Middleware {
	return func(ctx *Context) {
		timestamp := time.Now().UnixMilli()
		nonce := uuid.NewString()
		signature := calculateSignature(secret, signType, nonce, timestamp)

		if ctx.Request.Headers == nil {
			ctx.Request.Headers = make(map[string]string)
		}
		ctx.Request.Headers[defined.AppidHeader] = appID
		ctx.Request.Headers[defined.SignTypeHeader] = signType
		ctx.Request.Headers[defined.SignHeader] = signature
		ctx.Request.Headers[defined.SignTimestampHeader] = fmt.Sprintf("%d", timestamp)
		ctx.Request.Headers[defined.NonceHeader] = nonce

		ctx.Next()
	}
}

// URLValidationMiddleware URL 验证中间件
func URLValidationMiddleware() Middleware {
	return func(ctx *Context) {
		if ctx.Request.URL == "" {
			ctx.AbortWithError(errors.New("URL cannot be empty"))
			return
		}

		parsedURL, err := url.Parse(ctx.Request.URL)
		if err != nil {
			ctx.AbortWithError(fmt.Errorf("invalid URL format: %w", err))
			return
		}

		if parsedURL.Scheme == "" {
			ctx.AbortWithError(errors.New("URL must have a scheme (http:// or https://)"))
			return
		}

		if parsedURL.Host == "" {
			ctx.AbortWithError(errors.New("URL must have a host"))
			return
		}

		ctx.Next()
	}
}

// RequestSizeLimitMiddleware 请求体大小限制中间件
func RequestSizeLimitMiddleware(maxSize int64) Middleware {
	return func(ctx *Context) {
		if ctx.Request.Body != nil {
			var bodySize int64
			switch v := ctx.Request.Body.(type) {
			case []byte:
				bodySize = int64(len(v))
			case string:
				bodySize = int64(len(v))
			default:
				if jsonData, err := utils.NJson.Marshal(ctx.Request.Body); err == nil {
					bodySize = int64(len(jsonData))
				}
			}

			if bodySize > maxSize {
				ctx.AbortWithError(fmt.Errorf("request body size %d exceeds limit %d", bodySize, maxSize))
				return
			}
		}

		ctx.Next()
	}
}

// HeaderMiddleware 自定义请求头中间件
func HeaderMiddleware(headers map[string]string) Middleware {
	return func(ctx *Context) {
		if ctx.Request.Headers == nil {
			ctx.Request.Headers = make(map[string]string)
		}
		for k, v := range headers {
			ctx.Request.Headers[k] = v
		}
		ctx.Next()
	}
}

// TimeoutMiddleware 超时中间件
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(ctx *Context) {
		ctx.Request.Timeout = timeout
		ctx.Next()
	}
}

// ============================================================================
// 响应后置中间件
// ============================================================================

// StatusCodeCheckMiddleware 状态码检查中间件
//
// 参数:
//   - expectedCodes: 期望的状态码列表，为空时检查 2xx
func StatusCodeCheckMiddleware(expectedCodes ...int) Middleware {
	return func(ctx *Context) {
		ctx.Next()

		if ctx.Response == nil {
			return
		}

		if len(expectedCodes) == 0 {
			if ctx.Response.StatusCode < 200 || ctx.Response.StatusCode >= 300 {
				ctx.AddError(fmt.Errorf("unexpected status code: %d", ctx.Response.StatusCode))
			}
			return
		}

		for _, code := range expectedCodes {
			if ctx.Response.StatusCode == code {
				return
			}
		}

		ctx.AddError(fmt.Errorf("unexpected status code: %d, expected: %v", ctx.Response.StatusCode, expectedCodes))
	}
}

// CodeCheckMiddleware 业务状态码检查中间件
func CodeCheckMiddleware(expectedCode int64, codeField ...string) Middleware {
	return func(ctx *Context) {
		ctx.Next()

		if ctx.Response == nil || len(ctx.Response.Body) == 0 {
			return
		}

		field := "code"
		if len(codeField) > 0 {
			field = codeField[0]
		}

		gj := gjson.ParseBytes(ctx.Response.Body)
		codeG := gj.Get(field)
		if !codeG.Exists() || codeG.Int() != expectedCode {
			ctx.AddError(fmt.Errorf("business code error, expect: %d, got: %v", expectedCode, codeG.Value()))
		}
	}
}

// JSONValidationMiddleware JSON 验证中间件
func JSONValidationMiddleware() Middleware {
	return func(ctx *Context) {
		ctx.Next()

		if ctx.Response != nil && len(ctx.Response.Body) > 0 {
			if !gjson.ValidBytes(ctx.Response.Body) {
				ctx.AddError(errors.New("response is not valid JSON"))
			}
		}
	}
}

// ResponseSizeCheckMiddleware 响应体大小检查中间件
func ResponseSizeCheckMiddleware(maxSize int64) Middleware {
	return func(ctx *Context) {
		ctx.Next()

		if ctx.Response != nil && int64(len(ctx.Response.Body)) > maxSize {
			ctx.AddError(fmt.Errorf("response body size %d exceeds limit %d", len(ctx.Response.Body), maxSize))
		}
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

// calculateSignature 计算签名
func calculateSignature(secret, signType, nonce string, signTime int64) string {
	data := fmt.Sprintf("%d%s%s%s", signTime, secret, signType, nonce)
	hash := utils.MD5Sum(data)
	return hash
}
