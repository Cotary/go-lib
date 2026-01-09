package utils

import (
	"net/url"
)

// EncodeQueryParams 将 map 转换为 URL 查询字符串
func EncodeQueryParams(params map[string]string) string {
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	return values.Encode()
}
