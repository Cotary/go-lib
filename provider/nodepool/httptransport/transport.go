package httptransport

import (
	"context"
	"fmt"
	"strings"

	nethttp "github.com/Cotary/go-lib/net/http"
	"github.com/Cotary/go-lib/provider/nodepool"
)

// Transport 实现 nodepool.Transport 接口，基于 net/http 的 FastHTTP 客户端执行 HTTP 请求。
//
// 职责：
//   - 将 nodepool 的协议无关请求转换为具体的 HTTP 请求
//   - 标准化 URL 拼接（endpoint + path）
//   - 按优先级合并 Header（全局默认 < 节点级 < 请求级）
//   - 集成中间件链（日志、认证、重试等）
//
// 并发安全：Transport 创建后为只读配置，可安全地在多个 goroutine 中共享。
type Transport struct {
	opts Options
}

// New 创建 HTTP Transport。
//
// 默认使用 FastHTTPClient 作为底层客户端，自动记录请求日志。
// 可通过 Option 自定义客户端、Header、中间件、日志行为等。
//
// 使用示例:
//
//	transport := httptransport.New(
//	    httptransport.WithDefaultHeaders(map[string]string{
//	        "Content-Type": "application/json",
//	    }),
//	    httptransport.WithTimeout(10 * time.Second),
//	)
func New(opts ...Option) *Transport {
	o := defaultOptions()
	for _, fn := range opts {
		fn(&o)
	}
	return &Transport{opts: o}
}

// Execute 实现 nodepool.Transport 接口。
//
// req.Data 必须为 *HTTPRequest 类型，否则返回错误。
// endpoint 作为 base URL 与 HTTPRequest.Path 拼接成完整请求地址。
func (t *Transport) Execute(ctx context.Context, endpoint string, req *nodepool.Request) (*nodepool.Response, error) {
	httpReq, ok := req.Data.(*HTTPRequest)
	if !ok {
		return nil, fmt.Errorf("httptransport: req.Data 必须为 *HTTPRequest 类型，实际为 %T", req.Data)
	}

	url := buildURL(endpoint, httpReq.Path)
	headers := t.mergeHeaders(endpoint, httpReq.Headers)

	builder := nethttp.NewRequestBuilder(t.opts.client)
	if !t.opts.keepLog {
		builder.NoKeepLog()
	}
	if !t.opts.sendErrorMsg {
		builder.NoSendErrorMsg()
	}
	if t.opts.timeout > 0 {
		builder.SetTimeout(t.opts.timeout)
	}
	if len(t.opts.middlewares) > 0 {
		builder.Use(t.opts.middlewares...)
	}

	result := builder.Execute(ctx, httpReq.GetMethod(), url, httpReq.Query, httpReq.Body, headers)

	return buildNodepoolResponse(result)
}

// buildURL 拼接 endpoint（baseURL）和 path 为完整 URL。
// 如果 path 已包含 scheme（http:// 或 https://），则直接使用 path。
// 如果 path 为空，则直接使用 endpoint。
func buildURL(endpoint, path string) string {
	if path == "" {
		return endpoint
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return strings.TrimRight(endpoint, "/") + "/" + strings.TrimLeft(path, "/")
}

// mergeHeaders 按优先级合并 Header：全局默认 → 节点级 → 请求级。
// 高优先级的 Header 会覆盖低优先级的同名 Header。
func (t *Transport) mergeHeaders(endpoint string, reqHeaders map[string]string) map[string]string {
	merged := make(map[string]string, len(t.opts.headers)+len(reqHeaders))

	for k, v := range t.opts.headers {
		merged[k] = v
	}

	if nodeH, ok := t.opts.nodeHeaders[endpoint]; ok {
		for k, v := range nodeH {
			merged[k] = v
		}
	}

	for k, v := range reqHeaders {
		merged[k] = v
	}

	return merged
}

// buildNodepoolResponse 将 net/http 的 Result 转换为 nodepool.Response。
func buildNodepoolResponse(result *nethttp.Result) (*nodepool.Response, error) {
	if result.HasError() {
		httpResp := buildHTTPResponse(result.GetResponse())
		return &nodepool.Response{Data: httpResp}, result.Error
	}

	raw := result.GetResponse()
	httpResp := buildHTTPResponse(raw)

	return &nodepool.Response{
		Data: httpResp,
	}, nil
}

// buildHTTPResponse 从 net/http.Response 构建 HTTPResponse。
func buildHTTPResponse(raw *nethttp.Response) *HTTPResponse {
	if raw == nil {
		return &HTTPResponse{}
	}
	return &HTTPResponse{
		StatusCode: raw.StatusCode,
		Header:     raw.Header,
		Body:       raw.Body,
	}
}
