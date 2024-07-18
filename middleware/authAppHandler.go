package middleware

import (
	"fmt"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/response"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"time"
)

type SecretGetter func(appID string) string

func AuthMiddleware(getSecret SecretGetter) gin.HandlerFunc {
	return func(c *gin.Context) {
		appID := c.Request.Header.Get(defined.AppidHeader)
		signature := c.Request.Header.Get(defined.SignHeader)
		timestamp := c.Request.Header.Get(defined.SignTimestampHeader)

		// 这里应该使用你的方法来获取appID对应的secret
		secret := getSecret(appID)

		// 计算签名
		signatureCalculated := calculateSignature(c.Request.URL.Path, timestamp, secret)

		// 验证时间戳
		if !validateTimestamp(timestamp) {
			c.JSON(http.StatusOK, response.Error(c, e.NewHttpErr(e.SignTimeErr, nil)))
			c.Abort()
			return
		}
		// 验证签名
		if signature != signatureCalculated {
			c.JSON(http.StatusOK, response.Error(c, e.NewHttpErr(e.SignErr, nil)))
			c.Abort()
			return
		}

		c.Next()
	}
}

func calculateSignature(url, timestamp, secret string) string {
	data := fmt.Sprintf("%s%s%s", url, timestamp, secret)
	hash := utils.MD5(data)
	return hash
}

func validateTimestamp(timestamp string) bool {
	// 将时间戳转换为int64
	timestampInt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}

	// 获取当前时间的秒级时间戳
	now := time.Now().Unix()

	// 计算时间差的绝对值
	diff := now - timestampInt
	if diff < 0 {
		diff = -diff
	}

	// 验证时间差是否在5分钟以内
	return diff <= 5*60
}
