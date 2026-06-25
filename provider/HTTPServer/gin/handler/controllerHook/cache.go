package controllerHook

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/Cotary/go-lib/cache"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/notify"
	"github.com/gin-gonic/gin"
)

type cacheOpts struct {
	TTL        time.Duration
	HeaderKeys []string
}

// CacheOption CacheHook 的可选配置
type CacheOption func(*cacheOpts)

// WithTTL 设置缓存条目的过期时间；不传则使用 cache 实例的 DefaultTTL
func WithTTL(d time.Duration) CacheOption {
	return func(o *cacheOpts) { o.TTL = d }
}

// WithHeaders 指定需要纳入缓存 key 的请求 header 名称，避免不同 header 值命中同一缓存
func WithHeaders(keys ...string) CacheOption {
	return func(o *cacheOpts) { o.HeaderKeys = append(o.HeaderKeys, keys...) }
}

// CacheHook 创建一个响应缓存 Hook。
// cache hit 时短路返回，miss 时调用 next 并将结果存入缓存。
func CacheHook[R any](ch cache.Cache[R], opts ...CacheOption) Hook {
	var o cacheOpts
	for _, fn := range opts {
		fn(&o)
	}
	return func(c *gin.Context, req any, next func() (any, error)) (any, error) {
		ctx := c.Request.Context()

		reqJson, err := json.Marshal(req)
		if err != nil {
			notify.SendErrMessage(ctx, e.Err(err, "request cache marshal error"))
			return next()
		}
		key := c.Request.URL.Path + ":" + string(reqJson)
		for _, hk := range o.HeaderKeys {
			key += ":" + c.GetHeader(hk)
		}

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

		var setOpts []cache.Option
		if o.TTL > 0 {
			setOpts = append(setOpts, cache.WithTTL(o.TTL))
		}
		if setErr := ch.Set(ctx, key, result.(R), setOpts...); setErr != nil {
			notify.SendErrMessage(ctx, e.Err(setErr, "request set cache error"))
		}
		return result, nil
	}
}
