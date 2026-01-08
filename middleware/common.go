package middleware

import (
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/response"
	"github.com/gin-gonic/gin"
	"net/http"
)

func AbortWithError(c *gin.Context, err error) {
	c.JSON(http.StatusOK, response.Error(c, e.HTTPErrHandler(c, err)))
	c.Abort()
}
