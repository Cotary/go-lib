package http

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
		nonce := uuid.NewString()
		signature := calculateSignature(secret, signType, nonce, timestamp)
		// 添加签名字段到请求头
		headers[defined.AppidHeader] = appID
		headers[defined.SignTypeHeader] = signType
		headers[defined.SignHeader] = signature
		headers[defined.SignTimestampHeader] = fmt.Sprintf("%d", time.Now().UnixMilli())
		headers[defined.NonceHeader] = nonce

		return nil
	}
}

func calculateSignature(secret, signType, nonce string, signTime int64) string {
	data := fmt.Sprintf("%d%s%s%s", signTime, secret, signType, nonce)
	hash := utils.MD5(data)
	return hash
}
