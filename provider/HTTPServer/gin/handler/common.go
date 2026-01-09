package handler

import (
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/provider/HTTPServer/response"
	"github.com/gin-gonic/gin"
	"net/http"
)

func AbortWithError(c *gin.Context, err error) {
	c.JSON(http.StatusOK, response.Error(e.HTTPErrHandler(c, err)))
	c.Abort()
}
