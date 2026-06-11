package controllerHook

import (
	"encoding/json"
	"errors"

	"github.com/Cotary/go-lib/cache"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/notify"
	"github.com/gin-gonic/gin"
)

// CacheHook 创建一个响应缓存 Hook。
// cache hit 时短路返回，miss 时调用 next 并将结果存入缓存。
func CacheHook[R any](ch cache.Cache[R]) Hook {
	return func(c *gin.Context, req any, next func() (any, error)) (any, error) {
		ctx := c.Request.Context()

		reqJson, err := json.Marshal(req)
		if err != nil {
			notify.SendErrMessage(ctx, e.Err(err, "request cache marshal error"))
			return next()
		}
		key := c.Request.URL.Path + ":" + string(reqJson)

		resp, err := ch.Get(ctx, key)
		if err == nil {
			return resp, nil
		}
		if !errors.Is(err, cache.ErrNotFound) {
			notify.SendErrMessage(ctx, e.Err(err, "request cache get error"))
		}

		result, err := next()
		if err != nil {
			return nil, err
		}

		if setErr := ch.Set(ctx, key, result.(R)); setErr != nil {
			notify.SendErrMessage(ctx, e.Err(setErr, "request set cache error"))
		}
		return result, nil
	}
}
