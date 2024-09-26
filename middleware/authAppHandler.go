package middleware

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/cache"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/response"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"net/http"
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
type SignatureFunc func(c *gin.Context, signTime int64, secret string) (string, error)

func AuthMiddleware(conf AuthConf) gin.HandlerFunc {
	return func(c *gin.Context) {
		if conf.SecretGetter == nil {
			c.JSON(http.StatusOK, response.Error(c, e.NewHttpErr(e.ConfigErr, nil)))
			c.Abort()
			return
		}
		ctx := c.Request.Context()
		appID := c.Request.Header.Get(defined.AppidHeader)
		signature := c.Request.Header.Get(defined.SignHeader)
		signatureType := c.Request.Header.Get(defined.SignTypeHeader)
		timestamp := c.Request.Header.Get(defined.SignTimestampHeader) //ms
		signTime := utils.AnyToInt(timestamp)

		if signature == "" || timestamp == "" {
			c.JSON(http.StatusOK, response.Error(c, e.NewHttpErr(e.SignErr, errors.New(fmt.Sprintf("signature or timestamp not found")))))
			c.Abort()
			return
		}
		if conf.SignType != "" && conf.SignType != signatureType {
			c.JSON(http.StatusOK, response.Error(c, e.NewHttpErr(e.SignErr, errors.New(fmt.Sprintf("signature type error, expect %s, got %s", conf.SignType, signatureType)))))
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
		secret := conf.SecretGetter(ctx, appID)

		// 验证时间戳
		if !validateTimestamp(signTime, conf.Expire) {
			c.JSON(http.StatusOK, response.Error(c, e.NewHttpErr(e.SignTimeErr, errors.New(fmt.Sprintf("timestamp validate error")))))
			c.Abort()
			return
		}
		// 计算签名
		var validateFunc SignatureFunc = defaultValidateFunc
		if conf.SignatureFunc != nil {
			validateFunc = conf.SignatureFunc
		}
		signatureCalculated, err := validateFunc(c, signTime, secret)
		if err != nil {
			c.JSON(http.StatusOK, response.Error(c, e.NewHttpErr(e.SignErr, err)))
			c.Abort()
			return
		}

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

func defaultValidateFunc(c *gin.Context, signTime int64, secret string) (string, error) {
	url := c.Request.URL.Path
	data := fmt.Sprintf("%s%d%s", url, signTime, secret)
	hash := utils.MD5(data)
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
