package handler

import (
	"context"
	"net/http"

	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/notify"
	"github.com/Cotary/go-lib/provider/HTTPServer/response"
	"github.com/gin-gonic/gin"
)

// AbortWithError 是 HTTP 侧的错误收口点。
//
// 它是全项目唯一的错误出口：日志、告警、响应体组装三件事只在这里发生。
// 业务层只声明错误语义（e.NewBusinessErr），不碰日志和告警。
func AbortWithError(c *gin.Context, err error) {
	c.JSON(http.StatusOK, response.Error(HTTPErrHandler(c, err)))
	c.Abort()
}

// HTTPErrHandler 归一化错误并按级别触发告警。
//
// 【人写】告警只发生在这里：Level <= WarnLevel 才发，
// 避免业务正常分支（如"订单不存在"）刷屏。gRPC 侧是对称的 GRPCErrHandler。
func HTTPErrHandler(ctx context.Context, err error) *e.BusinessErr {
	bizErr := e.AsBusinessErr(err)
	if bizErr.Level <= e.WarnLevel {
		notify.SendErrMessage(ctx, err)
	}
	return bizErr
}
