package controllerHook

import (
	"reflect"

	e "github.com/Cotary/go-lib/err"
	"github.com/gin-gonic/gin"
)

type fieldValidator interface {
	Validate() error
}

var fieldValidatorType = reflect.TypeOf((*fieldValidator)(nil)).Elem()

// ValidateHook 扫描 req 所有字段，若字段实现了 Validate() error 接口则调用。
// 任一字段校验失败即返回 ParamErr，不再调用后续 hook。
// 典型用途：TimeRange 的 year_month/date 格式校验。
var ValidateHook Hook = func(c *gin.Context, req any, next func() (any, error)) (any, error) {
	if err := runValidate(req); err != nil {
		return nil, e.NewHttpErr(e.ParamErr, err).SetData(err.Error())
	}
	return next()
}

func runValidate(req any) error {
	v := reflect.ValueOf(req)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		fieldVal := v.Field(i)
		if !fieldVal.CanAddr() {
			continue
		}
		ptr := fieldVal.Addr()
		if !ptr.Type().Implements(fieldValidatorType) {
			continue
		}
		if err := ptr.Interface().(fieldValidator).Validate(); err != nil {
			return err
		}
	}
	return nil
}
