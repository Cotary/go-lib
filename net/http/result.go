package http

import (
	"context"
	"fmt"

	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/log"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
)

// ============================================================================
// 请求结果
// ============================================================================

// Result 请求结果
//
// 包含完整的请求/响应信息，支持链式调用
type Result struct {
	*Context
	keepLog      bool
	sendErrorMsg bool
}

// Log 手动记录请求日志
//
// 注意：如果 keepLog 为 true，日志会由 RequestLogMiddleware 自动记录。
// 此方法可用于在 NoKeepLog() 模式下手动记录日志。
func (r *Result) Log(logEntry log.Logger) *Result {
	ctx := r.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	logMap := r.getLogMap(ctx)
	if r.Error != nil {
		if logEntry != nil {
			logEntry.WithContext(ctx).WithFields(logMap).Error("HTTP Request")
		}
		if r.sendErrorMsg {
			e.SendMessage(ctx, errors.New("HTTP Request Error:"+utils.Json(logMap)))
		}
	} else {
		if logEntry != nil {
			logEntry.WithContext(ctx).WithFields(logMap).Info("HTTP Request")
		}
	}
	return r
}

// getLogMap 构建日志字段映射
func (r *Result) getLogMap(ctx context.Context) map[string]interface{} {
	logMap := map[string]interface{}{
		"Context ID": ctx.Value(defined.RequestID),
	}

	if r.Request != nil {
		logMap["Request URL"] = r.Request.URL
		logMap["Request Method"] = r.Request.Method
		logMap["Request Headers"] = r.Request.Headers
		logMap["Request Query"] = r.Request.Query
		if r.Request.Body != nil {
			logMap["Request Body"] = r.Request.Body
		}
	}

	if r.Response != nil {
		logMap["Response Status Code"] = r.Response.StatusCode
		logMap["Response Headers"] = r.Response.Header
		logMap["Response Body"] = string(r.Response.Body)

		if r.Response.Stats != nil {
			logMap["Total Time"] = r.Response.Stats.TotalTime.String()
		}
	}

	if r.Error != nil {
		logMap["Request Error"] = r.Error.Error()
	}

	// 添加中间件上下文中的额外信息
	if duration, ok := r.Get("request_duration"); ok {
		logMap["Duration"] = duration
	}
	if traceID, ok := r.Get("trace_id"); ok {
		logMap["Trace ID"] = traceID
	}

	return logMap
}

// HasError 检查是否有错误
func (r *Result) HasError() bool {
	return r.Error != nil
}

// GetResponse 获取原始响应
func (r *Result) GetResponse() *Response {
	return r.Response
}

// GetRequest 获取原始请求
func (r *Result) GetRequest() *Request {
	return r.Request
}

// parseJSON 解析响应 JSON（内部公共方法）
func (r *Result) parseJSON(path string) (string, error) {
	ctx := r.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	if r.Error != nil {
		return "", r.Error
	}
	if r.Response == nil {
		return "", errors.New("response is nil")
	}

	logMap := r.getLogMap(ctx)
	if r.Response.StatusCode < 200 || r.Response.StatusCode >= 300 {
		return "", errors.New("response status code error: " + utils.Json(logMap))
	}

	if !utils.IsJson(r.Response.Body) {
		return "", errors.New("response is not json: " + utils.Json(logMap))
	}

	if path == "" {
		return string(r.Response.Body), nil
	}

	value := gjson.ParseBytes(r.Response.Body).Get(path)
	if !value.Exists() {
		return "", errors.New(fmt.Sprintf("path not found: %s , response: %s", path, utils.Json(logMap)))
	}
	return value.String(), nil
}

// ParseTo 解析响应到目标指针（支持链式调用）
//
// 使用示例:
//
//	var user User
//	err := builder.Execute(ctx, "GET", url, nil, nil, nil).ParseTo("data.user", &user)
func (r *Result) ParseTo(path string, dest interface{}) error {
	respJson, err := r.parseJSON(path)
	if err != nil {
		return err
	}
	if dest == nil {
		return nil
	}
	if err = utils.AnyToAnyPtr(respJson, dest); err != nil {
		return e.Err(err, fmt.Sprintf("response parse error, response: %s", utils.Json(r.getLogMap(r.Ctx))))
	}
	return nil
}

// ============================================================================
// 响应解析工具函数
// ============================================================================

// Parse 解析响应到泛型类型 T
//
// 使用示例:
//
//	user, err := http.Parse[User](builder.Execute(ctx, "GET", url, nil, nil, nil), "data.user")
func Parse[T any](result *Result, path string) (T, error) {
	var zero T
	respJson, err := result.parseJSON(path)
	if err != nil {
		return zero, err
	}
	data, err := utils.AnyToAny[T](respJson)
	if err != nil {
		return zero, e.Err(err, fmt.Sprintf("response parse error, response: %s", utils.Json(result.getLogMap(result.Ctx))))
	}
	return data, nil
}

// MustParse 解析响应，失败时 panic
//
// 使用示例:
//
//	result := builder.Execute(ctx, "GET", url, nil, nil, nil)
//	user := http.MustParse[User](result, "data.user")
func MustParse[T any](result *Result, path string) T {
	data, err := Parse[T](result, path)
	if err != nil {
		panic(err)
	}
	return data
}
