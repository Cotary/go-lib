package handler

import (
	"context"
	"net/http"

	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/provider/HTTPServer/response"
	"github.com/gin-gonic/gin"
)

func AbortWithError(c *gin.Context, err error) {
	c.JSON(http.StatusOK, response.Error(HTTPErrHandler(c, err)))
	c.Abort()
}

func HTTPErrHandler(ctx context.Context, err error) *e.HttpErr {
	httpErr := e.AsHttpErr(err)
	if httpErr.Level <= e.WarnLevel {
		e.SendMessage(ctx, err)
	}
	return httpErr
}
