package controllerHook

import (
	"github.com/gin-gonic/gin"
)

// Hook 是 CD 管道中的处理函数（中间件模式）。
type Hook func(c *gin.Context, req any, next func() (any, error)) (any, error)
