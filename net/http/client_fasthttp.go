package http

import (
	"time"

	"github.com/Cotary/go-lib/common/utils"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
)

// ============================================================================
// FastHTTP 客户端
// ============================================================================

// DefaultFastHTTPClient 默认的 FastHTTP 客户端实例
var DefaultFastHTTPClient = NewFastHTTPClient()

// FastHTTPClient FastHTTP 客户端实现
type FastHTTPClient struct {
	client *fasthttp.Client
}

// NewFastHTTPClient 创建 FastHTTP 客户端
//
// 可选传入自定义的 fasthttp.Client，否则使用默认配置
func NewFastHTTPClient(args ...*fasthttp.Client) *FastHTTPClient {
	if len(args) > 0 {
		return &FastHTTPClient{
			client: args[0],
		}
	}
	return &FastHTTPClient{
		client: &fasthttp.Client{
			MaxConnsPerHost: 1000,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
		},
	}
}

// Do 执行 HTTP 请求
func (fc *FastHTTPClient) Do(req *Request) (*Response, error) {
	startTime := time.Now()

	fastReq := fasthttp.AcquireRequest()
	fastResp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(fastReq)
	defer fasthttp.ReleaseResponse(fastResp)

	if req.URL == "" {
		return nil, errors.New("URL cannot be empty")
	}

	fastReq.Header.SetMethod(req.Method)
	fastReq.SetRequestURI(req.URL)

	for key, values := range req.Query {
		for _, value := range values {
			fastReq.URI().QueryArgs().Add(key, value)
		}
	}

	for key, value := range req.Headers {
		fastReq.Header.Set(key, value)
	}

	if req.Body != nil {
		switch v := req.Body.(type) {
		case []byte:
			fastReq.SetBody(v)
		case string:
			fastReq.SetBodyString(v)
		default:
			fastReq.Header.SetContentType("application/json")
			if b, err := utils.NJson.Marshal(req.Body); err == nil {
				fastReq.SetBody(b)
			}
		}
	}

	if req.Timeout > 0 {
		fastReq.SetTimeout(req.Timeout)
	}

	err := fc.client.Do(fastReq, fastResp)
	endTime := time.Now()

	resp := &Response{
		Stats: &ResponseStats{
			StartTime: startTime,
			EndTime:   endTime,
			TotalTime: endTime.Sub(startTime),
		},
	}

	if err == nil {
		resp.StatusCode = fastResp.StatusCode()
		resp.Body = append([]byte(nil), fastResp.Body()...)
		resp.Header = make(map[string][]string)
		fastResp.Header.All()(func(key, value []byte) bool {
			resp.Header[string(key)] = []string{string(value)}
			return true
		})
	}

	return resp, err
}

// IsTimeout 判断是否为超时错误
func (fc *FastHTTPClient) IsTimeout(err error) bool {
	return errors.Is(err, fasthttp.ErrTimeout)
}

// ============================================================================
// 快捷函数
// ============================================================================

// FastHTTP 使用默认 FastHTTP 客户端创建请求构建器
//
// 推荐优先使用此方法，FastHTTP 性能更优
//
// 使用示例:
//
//	result := http.FastHTTP().Execute(ctx, "GET", url, nil, nil, nil)
//	user, err := http.Parse[User](result, "data")
func FastHTTP() *RequestBuilder {
	return NewRequestBuilder(DefaultFastHTTPClient)
}
