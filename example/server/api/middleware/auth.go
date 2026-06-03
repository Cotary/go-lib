//go:build ignore

// 本文件为使用示例，仅供参考，不可直接编译运行

package middleware

import (
	"net/http"

	"myproject/common/defined"
	"myproject/dao"

	"go-lib/provider/HTTPServer/response"

	"github.com/gin-gonic/gin"
)

// Auth Token 鉴权中间件。
// 从 Header 中取 Token，到 Redis 校验有效性。
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader(defined.HeaderToken)
		if token == "" {
			c.JSON(http.StatusUnauthorized, response.NewResponse(401, "Token required", nil))
			c.Abort()
			return
		}

		// 从 Redis 查询 Token 对应的用户信息
		ctx := c.Request.Context()
		userID, err := dao.Redis.Get(ctx, dao.Redis.Key(defined.RedisKeyUserToken+token)).Result()
		if err != nil || userID == "" {
			c.JSON(http.StatusUnauthorized, response.NewResponse(401, "Invalid token", nil))
			c.Abort()
			return
		}

		// 将用户信息写入 context，供后续 logic 使用
		c.Set("user_id", userID)
		c.Next()
	}
}
