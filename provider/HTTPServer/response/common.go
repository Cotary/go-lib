package response

import (
	"fmt"

	"github.com/Cotary/go-lib/common/appctx"
	"github.com/Cotary/go-lib/common/defined"
	e "github.com/Cotary/go-lib/err"
	"github.com/pkg/errors"
)

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
	var standardErr *e.HttpErr
	ok := errors.As(err, &standardErr)
	if !ok {
		standardErr = e.NewHttpErr(e.FailedErr, err)
	}

	msg := standardErr.Error()
	if appctx.Env() == defined.TEST && standardErr.Err != nil && standardErr.Err.Error() != "" {
		msg = fmt.Sprintf("%s: %s", standardErr.Error(), standardErr.Err.Error())
	}

	return NewResponse(standardErr.Code, msg, standardErr.Data)
}
