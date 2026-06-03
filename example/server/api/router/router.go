//go:build ignore

// 本文件为使用示例，仅供参考，不可直接编译运行

package router

import (
	"myproject/server/api/logic"
	"myproject/server/api/middleware"

	"go-lib/provider/HTTPServer/gin/handler"

	"github.com/gin-gonic/gin"
)

// RegisterRouter 注册所有路由。
//
// 路由命名规范：
//   - 统一前缀 /api
//   - 格式 /api/{模块}/{动作}，如 /api/order/create
//   - 模块名和动作名使用 kebab-case
//   - GET 请求用 ? 查询参数，POST 请求用 JSON body
func RegisterRouter(r *gin.Engine) {
	api := r.Group("/api")

	api.GET("/", func(c *gin.Context) {
		c.String(200, "Welcome to API Service")
	})

	// 订单模块
	order := api.Group("/order", middleware.Auth())
	order.GET("/detail", handler.CD(logic.OrderDetail))  // GET /api/order/detail?id=123
	order.GET("/list", handler.CD(logic.OrderList))      // GET /api/order/list?page=1&page_size=10&status=1
	order.POST("/create", handler.CD(logic.CreateOrder)) // POST /api/order/create  body: {"amount":"100","currency":"USD"}

	// 用户模块（示意）
	// user := api.Group("/user", middleware.Auth())
	// user.GET("/info", handler.CD(logic.UserInfo))
	// user.POST("/update", handler.CD(logic.UpdateUser))
}
