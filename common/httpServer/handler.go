package httpServer

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/google/uuid"
	"time"
)

type RequestHandler func(ctx context.Context, method *string, url *string, query map[string][]string, body any, headers map[string]string) error

func AuthAppHandler(appID, secret, signType string) RequestHandler {
	return func(ctx context.Context, method *string, url *string, query map[string][]string, body any, headers map[string]string) error {
		timestamp := time.Now().UnixMilli()
		signature := calculateSignature(*url, secret, signType, timestamp)
		// 添加签名字段到请求头
		headers[defined.AppidHeader] = appID
		headers[defined.SignTypeHeader] = signType
		headers[defined.SignHeader] = signature
		headers[defined.SignTimestampHeader] = fmt.Sprintf("%d", time.Now().UnixMilli())
		headers[defined.NonceHeader] = uuid.NewString()

		return nil
	}
}

func calculateSignature(url, secret, signType string, timestamp int64) string {
	data := fmt.Sprintf("%s%d%s%s", url, timestamp, secret, signType)
	hash := utils.MD5(data)
	return hash
}
