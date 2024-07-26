package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Cotary/go-lib/cache"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/response"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"io"
	"net/http"
)

type HandlerFuncWrapper func(c *gin.Context) (any, error)

func C(wrapper HandlerFuncWrapper) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := wrapper(c)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				c.JSON(http.StatusOK, response.Error(c, e.HTTPErrHandler(c, err)))
			}
			c.Abort()
			return
		}
		c.JSON(http.StatusOK, response.Success(c, resp))
	}
}

type ServiceFuncWrapper[T any, R any] func(c *gin.Context, req T) (resp R, err error)

type ControllerOptions struct {
	CacheStore  store.StoreInterface
	CacheExpire int64
}

func CD[T any, R any](wrapper ServiceFuncWrapper[T, R], options ...ControllerOptions) gin.HandlerFunc {
	return C(func(c *gin.Context) (any, error) {

		option := ControllerOptions{}
		if len(options) > 0 {
			option = options[0]
		}

		//ctx := c.Request.Context()
		req := new(T)

		//reload body
		bodyBytes, err := c.GetRawData()
		if err != nil {
			return nil, e.Err(err)
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		//bind
		if err = c.ShouldBind(req); err != nil {
			return nil, e.NewHttpErr(e.ParamErr, err)
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		//cache
		var cacheInstance *cache.BaseCache[R]
		var reqJson []byte
		if option.CacheExpire > 0 {
			reqJson, err = json.Marshal(*req)
			if err != nil {
				e.SendMessage(c, e.Err(err, "request cache marshal error"))
			}
			prefix := fmt.Sprintf("Request-%s", c.Request.URL.Path)
			cacheInstance = cache.StoreInstance[R](c,
				cache.Config{
					Prefix: prefix,
					Expire: option.CacheExpire,
				},
				option.CacheStore)

			resp, err := cacheInstance.Get(c, string(reqJson))
			if err != nil && err.Error() != store.NOT_FOUND_ERR {
				e.SendMessage(c, e.Err(err, "request cache get error"))
			} else {
				return resp, nil
			}
		}
		//logic
		resp, err := wrapper(c, *req)
		if err != nil {
			return nil, err
		}
		if option.CacheExpire > 0 {
			err = cacheInstance.Set(c, string(reqJson), resp)
			if err != nil {
				e.SendMessage(c, e.Err(err, "request set cache error"))
			}
		}

		return resp, nil
	})
}
