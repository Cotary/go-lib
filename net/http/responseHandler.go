package http

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
)

type ResponseHandler func(res *Result) error

var CodeCheckHandler = func(code int64, codeStr ...string) ResponseHandler {
	return func(res *Result) error {
		gj := gjson.ParseBytes(res.Response.Body)
		field := "code"
		if len(codeStr) > 0 {
			field = codeStr[0]
		}
		codeG := gj.Get(field)
		if !codeG.Exists() || codeG.Int() != code {
			return errors.New(fmt.Sprintf("code error,expect:%d", code))
		}
		return nil
	}
}
