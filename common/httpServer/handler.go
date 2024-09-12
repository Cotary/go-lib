package httpServer

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	"time"
)

type RequestHandler func(ctx context.Context, method *string, url *string, query map[string][]string, body interface{}, headers map[string]string) error

func AuthAppHandler(ctx context.Context, appID, secret string) RequestHandler {
	return func(ctx context.Context, method *string, url *string, query map[string][]string, body interface{}, headers map[string]string) error {
		timestamp := time.Now().UnixMilli()
		signature := calculateSignature(*url, secret, timestamp)
		// 添加签名字段到请求头
		headers[defined.AppidHeader] = appID
		headers[defined.SignHeader] = signature
		headers[defined.SignTimestampHeader] = fmt.Sprintf("%d", time.Now().UnixMilli())

		return nil
	}
}

func calculateSignature(url, secret string, timestamp int64) string {
	data := fmt.Sprintf("%s%d%s", url, timestamp, secret)
	hash := utils.MD5(data)
	return hash
}
