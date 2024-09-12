package httpServer

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
)

type CheckFunc func(res *RestyResult, gj gjson.Result) error

var CodeCheckFunc = func(code int64, codeStr ...string) CheckFunc {
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
