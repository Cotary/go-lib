package http

import (
	"context"
	"net"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
)

// ============================================================================
// Resty 客户端
// ============================================================================

// DefaultRestyClient 默认的 Resty 客户端实例
var DefaultRestyClient = NewRestyClient()

// RestyClient Resty 客户端实现
type RestyClient struct {
	client *resty.Client
}

// NewRestyClient 创建 Resty 客户端
//
// 可选传入自定义的 resty.Client，否则使用默认配置
func NewRestyClient(args ...*resty.Client) *RestyClient {
	if len(args) > 0 {
		return &RestyClient{
			client: args[0],
		}
	}
	return &RestyClient{
		client: resty.New(),
	}
}

// Do 执行 HTTP 请求
func (rc *RestyClient) Do(req *Request) (*Response, error) {
	if req.URL == "" {
		return nil, errors.New("URL cannot be empty")
	}

	startTime := time.Now()

	restyReq := rc.client.R().SetContext(req.Ctx)
	restyReq.SetQueryParamsFromValues(req.Query)
	restyReq.SetHeaders(req.Headers)

	if req.Body != nil {
		restyReq.SetBody(req.Body)
	}

	if req.Timeout > 0 {
		newCtx, cancel := context.WithTimeout(req.Ctx, req.Timeout)
		defer cancel()
		restyReq.SetContext(newCtx)
	}

	restyResp, err := restyReq.Execute(req.Method, req.URL)
	endTime := time.Now()

	resp := &Response{
		Stats: &ResponseStats{
			StartTime: startTime,
			EndTime:   endTime,
			TotalTime: endTime.Sub(startTime),
		},
	}

	if restyResp != nil {
		resp.StatusCode = restyResp.StatusCode()
		resp.Body = restyResp.Body()
		resp.Header = restyResp.Header()
	}

	return resp, err
}

// IsTimeout 判断是否为超时错误
func (rc *RestyClient) IsTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

// ============================================================================
// 快捷函数
// ============================================================================

// RestyHTTP 使用默认 Resty 客户端创建请求构建器
//
// 使用示例:
//
//	result := http.RestyHTTP().Execute(ctx, "GET", url, nil, nil, nil)
//	user, err := http.Parse[User](result, "data")
func RestyHTTP() *RequestBuilder {
	return NewRequestBuilder(DefaultRestyClient)
}
