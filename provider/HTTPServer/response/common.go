package response

import (
	"context"
	"fmt"

	"github.com/Cotary/go-lib/common/appctx"
	"github.com/Cotary/go-lib/common/defined"
	e "github.com/Cotary/go-lib/err"
)

// Response 是统一的 HTTP 响应体。
//
// Code 是业务码，Reason 是类别常量（与 gRPC 的 ErrorInfo.reason 同源）。
// 两者配合才能唯一确定一个错误：业务码在服务内有意义，
// 类别告诉客户端「这是哪一类错」。
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"msg"`
	Data    interface{} `json:"data"`
}

func NewResponse(code int, message string, data interface{}) *Response {
	return &Response{Code: code, Message: message, Data: data}
}

// Success Response
func Success(data any) *Response {
	return NewResponse(0, "success", data)
}

// Error Response
func Error(err error) *Response {
	return ErrorContext(context.Background(), err)
}

// ErrorContext 与 Error 相同，但优先从 ctx 读取身份（测试环境会暴露底层错误）。
func ErrorContext(ctx context.Context, err error) *Response {
	standardErr := e.AsBusinessErr(err)
	if standardErr == nil {
		standardErr = e.NewBusinessErr(e.FailedErr, err)
	}

	msg := standardErr.Error()
	if appctx.OrDefault(ctx).Env == defined.TEST && standardErr.Err != nil && standardErr.Err.Error() != "" {
		msg = fmt.Sprintf("%s: %s", standardErr.Error(), standardErr.Err.Error())
	}

	return &Response{
		Code:    standardErr.Code,
		Message: msg,
		Data:    standardErr.Data,
	}
}
