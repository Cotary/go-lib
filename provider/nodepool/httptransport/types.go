package httptransport

import (
	"net/http"
)

// HTTPRequest HTTP 请求描述，作为 nodepool.Request.Data 传入。
//
// Transport 会将 endpoint（baseURL）与 Path 拼接成完整 URL，
// 并按优先级合并 Header（Transport 默认 < 节点级 < 请求级）。
//
// 使用示例:
//
//	req := &nodepool.Request{
//	    Data: &httptransport.HTTPRequest{
//	        Method:  "POST",
//	        Path:    "/v1/chat/completions",
//	        Body:    map[string]any{"model": "gpt-4"},
//	        Headers: map[string]string{"X-Custom": "value"},
//	    },
//	}
type HTTPRequest struct {
	Method  string              // HTTP 方法（GET/POST/PUT/DELETE 等），为空时默认 GET
	Path    string              // 请求路径，与 endpoint 拼接；若包含 scheme 则直接作为完整 URL
	Query   map[string][]string // URL 查询参数
	Body    any                 // 请求体，非 []byte/string 类型会自动 JSON 序列化
	Headers map[string]string   // 本次请求的额外 Header，优先级最高
}

// GetMethod 返回 HTTP 方法，为空时默认 GET。
func (r *HTTPRequest) GetMethod() string {
	if r.Method == "" {
		return http.MethodGet
	}
	return r.Method
}

// HTTPResponse HTTP 响应数据，作为 nodepool.Response.Data 返回。
//
// 调用方通过类型断言获取：
//
//	httpResp := resp.Data.(*httptransport.HTTPResponse)
//	fmt.Println(httpResp.StatusCode, string(httpResp.Body))
type HTTPResponse struct {
	StatusCode int                 // HTTP 状态码
	Header     map[string][]string // 响应头
	Body       []byte              // 响应体原始字节
}

// String 返回响应体的字符串形式。
func (r *HTTPResponse) String() string {
	if len(r.Body) == 0 {
		return ""
	}
	return string(r.Body)
}
