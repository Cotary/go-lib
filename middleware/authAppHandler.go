package middleware

import (
	"fmt"
	"github.com/Cotary/go-lib/cache"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/response"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

type AuthConf struct {
	CacheStore   store.StoreInterface
	Expire       time.Duration
	SecretGetter SecretGetter
}
type SecretGetter func(appID string) string

func AuthMiddleware(conf AuthConf) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		appID := c.Request.Header.Get(defined.AppidHeader)
		signature := c.Request.Header.Get(defined.SignHeader)
		timestamp := c.Request.Header.Get(defined.SignTimestampHeader) //ms
		signTime := utils.AnyToInt(timestamp)

		if signature == "" || timestamp == "" {
			c.JSON(http.StatusOK, response.Error(c, e.NewHttpErr(e.SignErr, nil)))
			c.Abort()
			return
		}
		//检查sign重放
		var cacheInstance cache.Cache[int64]
		if conf.CacheStore != nil {
			cacheInstance = cache.StoreInstance(ctx,
				cache.Config[int64]{
					Prefix: "AuthSign",
					Expire: conf.Expire,
				},
				conf.CacheStore)
			_, err := cacheInstance.Get(ctx, signature)
			if err != nil {
				if err.Error() != store.NOT_FOUND_ERR {
					e.SendMessage(ctx, e.Err(err, "AuthSign cache get error"))
				}
			} else {
				c.JSON(http.StatusOK, response.Error(c, e.NewHttpErr(e.SignReplayErr, nil)))
				c.Abort()
				return
			}
		}

		// 这里应该使用你的方法来获取appID对应的secret
		secret := conf.SecretGetter(appID)

		// 验证时间戳

		if !validateTimestamp(signTime, conf.Expire) {
			c.JSON(http.StatusOK, response.Error(c, e.NewHttpErr(e.SignTimeErr, nil)))
			c.Abort()
			return
		}
		// 计算签名
		signatureCalculated := calculateSignature(c.Request.URL.Path, timestamp, secret)

		// 验证签名
		if signature != signatureCalculated {
			c.JSON(http.StatusOK, response.Error(c, e.NewHttpErr(e.SignErr, nil)))
			c.Abort()
			return
		}

		if conf.CacheStore != nil {
			err := cacheInstance.Set(ctx, signature, signTime)
			if err != nil {
				e.SendMessage(ctx, e.Err(err, "AuthSign set cache error"))
			}
		}
		c.Next()
	}
}

func calculateSignature(url, timestamp, secret string) string {
	data := fmt.Sprintf("%s%s%s", url, timestamp, secret)
	hash := utils.MD5(data)
	return hash
}

func validateTimestamp(timestamp int64, expire time.Duration) bool {
	now := time.Now().UnixMilli()
	diff := now - timestamp
	if diff < 0 {
		diff = -diff
	}
	return diff <= expire.Milliseconds()
}
