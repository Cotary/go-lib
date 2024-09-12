package httpServer

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
)

type ResponseHandler func(res *RestyResult, gj gjson.Result) error

var CodeCheckHandler = func(code int64, codeStr ...string) ResponseHandler {
	return func(res *RestyResult, gj gjson.Result) error {
		field := "code"
		if len(codeStr) > 0 {
			field = codeStr[0]
		}
		if gj.Get(field).Int() != code {
			return errors.New(fmt.Sprintf("code error,expect:%d", code))
		}
		return nil
	}
}
