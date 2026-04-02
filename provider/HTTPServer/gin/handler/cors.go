package handler

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

func CorsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")

		// 1. 如果没有 Origin，尝试从 Referer 提取（适配某些特殊请求）
		if origin == "" {
			referer := c.Request.Header.Get("Referer")
			if referer != "" {
				if u, err := url.Parse(referer); err == nil {
					// 提取 protocol + host (e.g., http://localhost:3000)
					origin = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
				}
			}
		}

		// 2. 动态设置 Access-Control-Allow-Origin
		if origin != "" {
			// 只有动态回显具体的 Origin，才能支持 Allow-Credentials: true
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
		} else {
			// 如果实在拿不到 Origin 和 Referer（比如 Postman 直接调）
			// 设为 *，但注意此时前端如果带 Cookie 会报错（测试环境通常能接受）
			c.Header("Access-Control-Allow-Origin", "*")
		}

		// 3. 允许所有常用的方法和 Header
		c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, PATCH")
		c.Header("Access-Control-Allow-Headers", "*")
		c.Header("Access-Control-Expose-Headers", "*")
		// 4. 预检请求缓存（避免频繁发送 OPTIONS）
		c.Header("Access-Control-Max-Age", "86400")

		// 5. 处理预检请求
		if method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
