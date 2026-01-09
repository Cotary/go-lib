package handler

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/cache"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/gin-gonic/gin"
	"time"
)

type AuthConf struct {
	CacheStore    store.StoreInterface
	Expire        time.Duration
	SignType      string
	SecretGetter  SecretGetter
	SignatureFunc SignatureFunc
}
type SecretGetter func(ctx context.Context, appID string) string
type SignatureFunc func(c *gin.Context, signTime int64, secret, signType, nonce string) (string, error)

func AuthMiddleware(conf AuthConf) gin.HandlerFunc {
	return func(c *gin.Context) {
		if conf.SecretGetter == nil {
			AbortWithError(c, e.ConfigErr)
			return
		}
		ctx := c.Request.Context()
		appID := c.Request.Header.Get(defined.AppidHeader)
		signature := c.Request.Header.Get(defined.SignHeader)
		signatureType := c.Request.Header.Get(defined.SignTypeHeader)
		nonce := c.Request.Header.Get(defined.NonceHeader)
		timestamp := c.Request.Header.Get(defined.SignTimestampHeader) //ms
		signTime := utils.AnyToInt(timestamp)

		if signature == "" || timestamp == "" {
			AbortWithError(c, e.SignErr)
			return
		}
		if conf.SignType != "" && conf.SignType != signatureType {
			AbortWithError(c, e.SignErr)
			return
		}
		//检查sign重放
		var cacheInstance cache.Cache[int64]
		if conf.CacheStore != nil {
			cacheInstance = cache.StoreInstance(
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
				AbortWithError(c, e.SignReplayErr)
				return
			}
		}

		// 这里应该使用你的方法来获取appID对应的secret
		secret := conf.SecretGetter(ctx, appID)
		if secret == "" {
			AbortWithError(c, e.SignErr)
			return
		}

		// 验证时间戳
		if !validateTimestamp(signTime, conf.Expire) {
			AbortWithError(c, e.SignTimeErr)
			return
		}
		// 计算签名
		var validateFunc SignatureFunc = defaultValidateFunc
		if conf.SignatureFunc != nil {
			validateFunc = conf.SignatureFunc
		}
		signatureCalculated, err := validateFunc(c, signTime, secret, signatureType, nonce)
		if err != nil {
			AbortWithError(c, e.SignErr)
			return
		}

		// 验证签名
		if signature != signatureCalculated {
			AbortWithError(c, e.SignErr)
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

func defaultValidateFunc(c *gin.Context, signTime int64, secret, signType, nonce string) (string, error) {
	data := fmt.Sprintf("%d%s%s%s", signTime, secret, signType, nonce)
	hash := utils.MD5Sum(data)
	return hash, nil
}

func validateTimestamp(timestamp int64, expire time.Duration) bool {
	now := time.Now().UnixMilli()
	diff := now - timestamp
	if diff < 0 {
		diff = -diff
	}
	return diff <= expire.Milliseconds()
}
