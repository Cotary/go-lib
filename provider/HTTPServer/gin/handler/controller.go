package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Cotary/go-lib/cache"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/notify"
	"github.com/Cotary/go-lib/provider/HTTPServer/gin/exporter"
	"github.com/Cotary/go-lib/provider/HTTPServer/gin/utils"
	"github.com/Cotary/go-lib/provider/HTTPServer/response"
	"github.com/gin-gonic/gin"
)

type HandlerFuncWrapper func(c *gin.Context) (any, error)

func C(wrapper HandlerFuncWrapper) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := wrapper(c)
		if err != nil {
			AbortWithError(c, err)
			return
		}
		// 如果 wrapper 内部已自行写入响应，则不再重复输出
		if c.Writer.Written() {
			return
		}
		// 检查是否为导出请求
		if exporter.IsDownload(c) {
			if exportErr := exporter.Export(c, resp); exportErr != nil {
				AbortWithError(c, exportErr)
				return
			}
			return
		}
		c.JSON(http.StatusOK, response.Success(resp))
	}
}

type ServiceFuncWrapper[T any, R any] func(c *gin.Context, req T) (resp R, err error)

type ControllerOptions[R any] struct {
	Cache cache.Cache[R]
}

func CD[T any, R any](wrapper ServiceFuncWrapper[T, R], options ...ControllerOptions[R]) gin.HandlerFunc {
	return C(func(c *gin.Context) (any, error) {
		var option ControllerOptions[R]
		if len(options) > 0 {
			option = options[0]
		}

		ctx := c.Request.Context()
		req := new(T)

		bodyBytes, err := utils.GetRequestBody(c)
		if err != nil {
			return nil, e.Err(err)
		}

		if err = c.ShouldBind(req); err != nil {
			return nil, e.NewHttpErr(e.ParamErr, err).SetData(err.Error())
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		var cacheKey string
		if option.Cache != nil {
			reqJson, err := json.Marshal(*req)
			if err != nil {
				notify.SendErrMessage(ctx, e.Err(err, "request cache marshal error"))
			} else {
				cacheKey = c.Request.URL.Path + ":" + string(reqJson)
				resp, err := option.Cache.Get(ctx, cacheKey)
				if err == nil {
					return resp, nil
				}
				if !errors.Is(err, cache.ErrNotFound) {
					notify.SendErrMessage(ctx, e.Err(err, "request cache get error"))
				}
			}
		}

		resp, err := wrapper(c, *req)
		if err != nil {
			return nil, err
		}

		if option.Cache != nil && cacheKey != "" {
			if setErr := option.Cache.Set(ctx, cacheKey, resp); setErr != nil {
				notify.SendErrMessage(ctx, e.Err(setErr, "request set cache error"))
			}
		}

		return resp, nil
	})
}
