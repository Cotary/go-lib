package http

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
)

type ResponseHandler func(res *Result) error

// CodeCheckHandler 检查响应中的业务状态码
var CodeCheckHandler = func(code int64, codeStr ...string) ResponseHandler {
	return func(res *Result) error {
		gj := gjson.ParseBytes(res.Response.Body)
		field := "code"
		if len(codeStr) > 0 {
			field = codeStr[0]
		}
		codeG := gj.Get(field)
		if !codeG.Exists() || codeG.Int() != code {
			return errors.New(fmt.Sprintf("code error,expect:%d", code))
		}
		return nil
	}
}

// StatusCodeCheckHandler 检查HTTP状态码
func StatusCodeCheckHandler(expectedCodes ...int) ResponseHandler {
	return func(res *Result) error {
		if len(expectedCodes) == 0 {
			// 默认检查2xx状态码
			if res.Response.StatusCode < 200 || res.Response.StatusCode >= 300 {
				return errors.New(fmt.Sprintf("unexpected status code: %d", res.Response.StatusCode))
			}
			return nil
		}

		for _, code := range expectedCodes {
			if res.Response.StatusCode == code {
				return nil
			}
		}

		return errors.New(fmt.Sprintf("unexpected status code: %d, expected: %v", res.Response.StatusCode, expectedCodes))
	}
}

// ResponseSizeCheckHandler 检查响应体大小
func ResponseSizeCheckHandler(maxSize int64) ResponseHandler {
	return func(res *Result) error {
		if int64(len(res.Response.Body)) > maxSize {
			return errors.New(fmt.Sprintf("response body size %d exceeds limit %d", len(res.Response.Body), maxSize))
		}
		return nil
	}
}

// JSONValidationHandler 验证响应是否为有效JSON
func JSONValidationHandler() ResponseHandler {
	return func(res *Result) error {
		if !gjson.ValidBytes(res.Response.Body) {
			return errors.New("response is not valid JSON")
		}
		return nil
	}
}

// RetryOnErrorHandler 根据错误类型决定是否重试
func RetryOnErrorHandler(retryableErrors ...string) ResponseHandler {
	return func(res *Result) error {
		if res.Error == nil {
			return nil
		}

		errorMsg := res.Error.Error()
		for _, retryableError := range retryableErrors {
			if errorMsg == retryableError {
				// 标记为可重试错误
				return errors.New(fmt.Sprintf("retryable error: %s", errorMsg))
			}
		}

		return res.Error
	}
}
