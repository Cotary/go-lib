package controllerHook

import (
	"reflect"

	"github.com/gin-gonic/gin"
)

// AutoBinder 支持 prefix tag 自动绑定的字段类型接口。
// 结构体字段类型实现此接口后，AutoBindHook 会在 ShouldBind 后
// 扫描 `prefix` tag，将 tag 值和 getter 传入 AutoBind，
// 由具体类型自行决定绑定逻辑。
type AutoBinder interface {
	AutoBind(getter func(string) string, prefix string)
}

// AutoBindHook 扫描 req 中带 `prefix` tag 且实现了 AutoBinder 接口的字段，
// 调用其 AutoBind 方法完成前缀参数绑定。无匹配字段时为 no-op。
var AutoBindHook Hook = func(c *gin.Context, req any, next func() (any, error)) (any, error) {
	runAutoBind(func(key string) string {
		if v, ok := c.GetQuery(key); ok {
			return v
		}
		if v, ok := c.GetPostForm(key); ok {
			return v
		}
		return ""
	}, req)
	return next()
}

var autoBinder = reflect.TypeOf((*AutoBinder)(nil)).Elem()

func runAutoBind(getter func(string) string, req any) {
	v := reflect.ValueOf(req)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		prefix := field.Tag.Get("prefix")
		if prefix == "" {
			continue
		}
		fieldVal := v.Field(i)
		if !fieldVal.CanAddr() {
			continue
		}
		ptr := fieldVal.Addr()
		if !ptr.Type().Implements(autoBinder) {
			continue
		}
		ptr.Interface().(AutoBinder).AutoBind(getter, prefix)
	}
}
