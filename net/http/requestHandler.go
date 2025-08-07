package http

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/google/uuid"
)

type RequestHandler func(ctx context.Context, method *string, url *string, query map[string][]string, body any, headers map[string]string) error

// AuthAppHandler 添加应用认证头
func AuthAppHandler(appID, secret, signType string) RequestHandler {
	return func(ctx context.Context, method *string, url *string, query map[string][]string, body any, headers map[string]string) error {
		timestamp := time.Now().UnixMilli()
		nonce := uuid.NewString()
		signature := calculateSignature(secret, signType, nonce, timestamp)
		// 添加签名字段到请求头
		headers[defined.AppidHeader] = appID
		headers[defined.SignTypeHeader] = signType
		headers[defined.SignHeader] = signature
		headers[defined.SignTimestampHeader] = fmt.Sprintf("%d", timestamp)
		headers[defined.NonceHeader] = nonce

		return nil
	}
}

// URLValidationHandler 验证URL格式
func URLValidationHandler() RequestHandler {
	return func(ctx context.Context, method *string, urlStr *string, query map[string][]string, body any, headers map[string]string) error {
		if *urlStr == "" {
			return fmt.Errorf("URL cannot be empty")
		}

		parsedURL, err := url.Parse(*urlStr)
		if err != nil {
			return fmt.Errorf("invalid URL format: %w", err)
		}

		if parsedURL.Scheme == "" {
			return fmt.Errorf("URL must have a scheme (http:// or https://)")
		}

		if parsedURL.Host == "" {
			return fmt.Errorf("URL must have a host")
		}

		return nil
	}
}

// RequestSizeLimitHandler 限制请求体大小
func RequestSizeLimitHandler(maxSize int64) RequestHandler {
	return func(ctx context.Context, method *string, url *string, query map[string][]string, body any, headers map[string]string) error {
		if body == nil {
			return nil
		}

		var bodySize int64
		switch v := body.(type) {
		case []byte:
			bodySize = int64(len(v))
		case string:
			bodySize = int64(len(v))
		default:
			// 对于复杂类型，序列化后检查大小
			if jsonData, err := utils.NJson.Marshal(body); err == nil {
				bodySize = int64(len(jsonData))
			} else {
				// 如果序列化失败，假设大小合理
				return nil
			}
		}

		if bodySize > maxSize {
			return fmt.Errorf("request body size %d exceeds limit %d", bodySize, maxSize)
		}

		return nil
	}
}

func calculateSignature(secret, signType, nonce string, signTime int64) string {
	data := fmt.Sprintf("%d%s%s%s", signTime, secret, signType, nonce)
	hash := utils.MD5(data)
	return hash
}
