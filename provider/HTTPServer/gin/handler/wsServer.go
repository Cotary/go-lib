package handler

import (
	"github.com/Cotary/go-lib/net/ws"
	"github.com/gin-gonic/gin"
)

// ServeGin 返回 gin 的 WebSocket 处理器
func ServeGin(wsServer *ws.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		wsServer.ServeHTTP(c.Writer, c.Request)
	}
}
