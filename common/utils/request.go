package utils

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"net/url"
)

func GetParam(c *gin.Context, paramName string) string {
	// try to get the parameter from the URL
	paramValue := c.Param(paramName)

	// if the parameter is not in the URL, try to get it from the form data
	if paramValue == "" {
		paramValue = c.PostForm(paramName)
	}

	// if the parameter is still not found, try to get it from the request body
	if paramValue == "" {
		requestBody, err := io.ReadAll(c.Request.Body)
		if err != nil {
			return ""
		}
		//再把数据装回去
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		// unmarshal the request body into a map[string]interface{}
		var requestBodyMap map[string]interface{}
		if err = json.Unmarshal(requestBody, &requestBodyMap); err != nil {
			return ""
		}

		// try to get the parameter from the request body
		if value, ok := requestBodyMap[paramName]; ok {
			paramValue = value.(string)
		}

	}

	return paramValue
}

// ClientIP 获取客户端ip
func ClientIP(c *gin.Context) string {
	ip := c.ClientIP()
	return ip
}

func FormatUrl(params map[string]string) string {
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}

	queryString := values.Encode()
	return queryString
}

func GetFullURL(c *gin.Context) string {
	req := c.Request
	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	host := req.Host
	fullURL := scheme + "://" + host + req.RequestURI
	return fullURL
}
