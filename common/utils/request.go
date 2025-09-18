package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/gin-gonic/gin"
	"io"
	"net/url"
)

// GetParam 从 URL 参数、表单或 JSON 请求体中获取指定参数值
func GetParam(ctx *gin.Context, paramName string) string {
	// 1. URL 参数
	if val := ctx.Param(paramName); val != "" {
		return val
	}

	// 2. 表单参数
	if val := ctx.PostForm(paramName); val != "" {
		return val
	}

	// 3. JSON 请求体
	bodyBytes, err := GetRequestBody(ctx)
	if err != nil || len(bodyBytes) == 0 {
		return ""
	}

	var bodyMap map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &bodyMap); err != nil {
		return ""
	}

	if val, ok := bodyMap[paramName]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// ClientIP 获取客户端 IP
func ClientIP(ctx *gin.Context) string {
	return ctx.ClientIP()
}

// EncodeQueryParams 将 map 转换为 URL 查询字符串
func EncodeQueryParams(params map[string]string) string {
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	return values.Encode()
}

// FullRequestURL 获取完整请求 URL（含协议、主机、路径和查询参数）
func FullRequestURL(ctx *gin.Context) string {
	req := ctx.Request
	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + req.Host + req.RequestURI
}

// GetRequestBody 获取请求体字节数据（缓存到 Context，避免多次读取）
func GetRequestBody(ctx *gin.Context) ([]byte, error) {
	if cached, ok := ctx.Value(defined.RequestBody).([]byte); ok && cached != nil {
		return cached, nil
	}

	bodyBytes, err := ctx.GetRawData()
	if err != nil {
		return nil, err
	}

	// 重新放回 Body，保证后续还能读取
	ctx.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	// 缓存到 Context
	ctx.Request = ctx.Request.WithContext(
		context.WithValue(ctx.Request.Context(), defined.RequestBody, bodyBytes),
	)
	return bodyBytes, nil
}
