package handler

import (
	"bytes"
	"io"
	"net/http"

	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/provider/HTTPServer/gin/exporter"
	"github.com/Cotary/go-lib/provider/HTTPServer/gin/handler/controllerHook"
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

func CD[T any, R any](wrapper ServiceFuncWrapper[T, R], hooks ...controllerHook.Hook) gin.HandlerFunc {
	return C(func(c *gin.Context) (any, error) {
		req := new(T)

		bodyBytes, err := utils.GetRequestBody(c)
		if err != nil {
			return nil, e.Err(err)
		}

		if err = c.ShouldBind(req); err != nil {
			return nil, e.NewHttpErr(e.ParamErr, err).SetData(err.Error())
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// 构建 Hook 链：AutoBind → Validate → 用户 hooks → handler
		allHooks := make([]controllerHook.Hook, 0, 2+len(hooks))
		allHooks = append(allHooks, controllerHook.AutoBindHook)
		allHooks = append(allHooks, controllerHook.ValidateHook)
		allHooks = append(allHooks, hooks...)

		chain := func() (any, error) {
			return wrapper(c, *req)
		}
		for i := len(allHooks) - 1; i >= 0; i-- {
			hook := allHooks[i]
			innerNext := chain
			chain = func() (any, error) {
				return hook(c, req, innerNext)
			}
		}
		return chain()
	})
}
