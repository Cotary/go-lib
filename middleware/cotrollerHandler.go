package middleware

import (
	"bytes"
	"context"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/response"
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

func CD[T any, R any](wrapper ServiceFuncWrapper[T, R]) gin.HandlerFunc {
	return C(func(c *gin.Context) (any, error) {
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
		//logic
		resp, err := wrapper(c, *req)
		if err != nil {
			return nil, err
		}

		return resp, nil
	})
}
