package http

import (
	"fmt"
	"net/url"
	"time"

	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/google/uuid"
)

type RequestHandler func(req *Request) error

// AuthAppHandler 添加应用认证头
func AuthAppHandler(appID, secret, signType string) RequestHandler {
	return func(req *Request) error {
		timestamp := time.Now().UnixMilli()
		nonce := uuid.NewString()
		signature := calculateSignature(secret, signType, nonce, timestamp)
		// 添加签名字段到请求头
		req.Headers[defined.AppidHeader] = appID
		req.Headers[defined.SignTypeHeader] = signType
		req.Headers[defined.SignHeader] = signature
		req.Headers[defined.SignTimestampHeader] = fmt.Sprintf("%d", timestamp)
		req.Headers[defined.NonceHeader] = nonce

		return nil
	}
}

// URLValidationHandler 验证URL格式
func URLValidationHandler() RequestHandler {
	return func(req *Request) error {
		if req.URL == "" {
			return fmt.Errorf("URL cannot be empty")
		}

		parsedURL, err := url.Parse(req.URL)
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
	return func(req *Request) error {
		if req.Body == nil {
			return nil
		}

		var bodySize int64
		switch v := req.Body.(type) {
		case []byte:
			bodySize = int64(len(v))
		case string:
			bodySize = int64(len(v))
		default:
			// 对于复杂类型，序列化后检查大小
			if jsonData, err := utils.NJson.Marshal(req.Body); err == nil {
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
