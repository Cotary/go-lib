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

// Result 泛型请求结果
//
// 包含完整的请求/响应信息，支持链式调用和泛型解析
type Result[T any] struct {
	*Context
	keepLog      bool
	sendErrorMsg bool
}

// Log 手动记录请求日志
//
// 注意：如果 keepLog 为 true，日志会由 RequestLogMiddleware 自动记录。
// 此方法可用于在 NoKeepLog() 模式下手动记录日志。
func (r *Result[T]) Log(logEntry log.Logger) *Result[T] {
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
func (r *Result[T]) getLogMap(ctx context.Context) map[string]interface{} {
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

// Parse 解析响应到泛型类型 T
//
// 参数:
//   - path: JSON 路径（如 "data.user"），空字符串表示解析整个响应
//
// 返回:
//   - T: 解析后的数据
//   - error: 解析错误
func (r *Result[T]) Parse(path string) (T, error) {
	var zero T
	ctx := r.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	if r.Error != nil {
		return zero, r.Error
	}
	if r.Response == nil {
		return zero, errors.New("response is nil")
	}

	logMap := r.getLogMap(ctx)
	if r.Response.StatusCode < 200 || r.Response.StatusCode >= 300 {
		return zero, errors.New("response status code error: " + utils.Json(logMap))
	}

	isJson := utils.IsJson(r.Response.Body)
	if !isJson {
		return zero, errors.New("response is not json: " + utils.Json(logMap))
	}

	// 使用 gjson 解析
	var respJson string
	if path != "" {
		gj := gjson.ParseBytes(r.Response.Body)
		value := gj.Get(path)
		if !value.Exists() {
			return zero, errors.New(fmt.Sprintf("path not found: %s , response: %s", path, utils.Json(logMap)))
		}
		respJson = value.String()
	} else {
		respJson = string(r.Response.Body)
	}

	// 使用泛型转换
	data, err := utils.AnyToAny[T](respJson)
	if err != nil {
		return zero, e.Err(err, fmt.Sprintf("response parse error, response: %s", utils.Json(logMap)))
	}
	return data, nil
}

// MustParse 解析响应，失败时 panic
func (r *Result[T]) MustParse(path string) T {
	data, err := r.Parse(path)
	if err != nil {
		panic(err)
	}
	return data
}

// HasError 检查是否有错误
func (r *Result[T]) HasError() bool {
	return r.Error != nil
}

// GetResponse 获取原始响应
func (r *Result[T]) GetResponse() *Response {
	return r.Response
}

// GetRequest 获取原始请求
func (r *Result[T]) GetRequest() *Request {
	return r.Request
}
