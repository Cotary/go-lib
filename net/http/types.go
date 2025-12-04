package http

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// ============================================================================
// HTTP 方法验证
// ============================================================================

// validHTTPMethods 有效的 HTTP 方法集合
var validHTTPMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodPost:    true,
	http.MethodPut:     true,
	http.MethodDelete:  true,
	http.MethodPatch:   true,
	http.MethodHead:    true,
	http.MethodOptions: true,
}

// isValidHTTPMethod 验证 HTTP 方法是否有效
func isValidHTTPMethod(method string) bool {
	return validHTTPMethods[method]
}

// ============================================================================
// 请求结构体
// ============================================================================

// Request HTTP 请求结构体
type Request struct {
	Ctx     context.Context     // 请求上下文
	Method  string              // HTTP 方法
	URL     string              // 请求 URL
	Query   map[string][]string // 查询参数
	Body    interface{}         // 请求体
	Headers map[string]string   // 请求头
	Timeout time.Duration       // 超时时间
}

// ============================================================================
// 响应结构体
// ============================================================================

// Response HTTP 响应结构体
type Response struct {
	StatusCode int                 // HTTP 状态码
	Header     map[string][]string // 响应头
	Body       []byte              // 响应体
	Stats      *ResponseStats      // 统计数据
}

// String 返回响应体字符串
func (r *Response) String() string {
	if len(r.Body) == 0 {
		return ""
	}
	return strings.TrimSpace(string(r.Body))
}

// ResponseStats HTTP 响应统计信息
type ResponseStats struct {
	TotalTime time.Duration // 总执行时间
	StartTime time.Time     // 请求开始时间
	EndTime   time.Time     // 请求结束时间
}

// ============================================================================
// 客户端接口
// ============================================================================

// IClient HTTP 客户端接口
//
// 定义了不同 HTTP 客户端实现（如 fasthttp、resty）的契约
type IClient interface {
	// Do 执行 HTTP 请求
	Do(request *Request) (*Response, error)
	// IsTimeout 判断错误是否为超时错误
	IsTimeout(err error) bool
}
