package http

import (
	"context"
	"net"
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
//
// 支持通过 req.Ctx 取消请求。由于 FastHTTP 不原生支持 context，
// 当 context 被取消时立即返回 context 错误，后台 goroutine 自行清理资源。
func (fc *FastHTTPClient) Do(req *Request) (*Response, error) {
	startTime := time.Now()

	if req.URL == "" {
		return nil, errors.New("URL cannot be empty")
	}

	fastReq := fasthttp.AcquireRequest()
	fastResp := fasthttp.AcquireResponse()

	fc.setupRequest(fastReq, req)

	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	if ctx.Done() == nil {
		err := fc.client.Do(fastReq, fastResp)
		resp := fc.buildResponse(startTime, fastResp, err)
		fasthttp.ReleaseRequest(fastReq)
		fasthttp.ReleaseResponse(fastResp)
		return resp, err
	}

	return fc.doWithContext(ctx, startTime, fastReq, fastResp)
}

// setupRequest 将通用 Request 设置到 fasthttp.Request 上
func (fc *FastHTTPClient) setupRequest(fastReq *fasthttp.Request, req *Request) {
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
}

// buildResponse 从 fasthttp.Response 构建通用 Response
func (fc *FastHTTPClient) buildResponse(startTime time.Time, fastResp *fasthttp.Response, err error) *Response {
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

	return resp
}

// doWithContext 在独立 goroutine 中执行 fasthttp 请求，同时监听 context 取消。
// context 取消时立即返回，后台 goroutine 完成后自行释放资源。
func (fc *FastHTTPClient) doWithContext(ctx context.Context, startTime time.Time, fastReq *fasthttp.Request, fastResp *fasthttp.Response) (*Response, error) {
	ch := make(chan error, 1)

	go func() {
		ch <- fc.client.Do(fastReq, fastResp)
	}()

	select {
	case err := <-ch:
		resp := fc.buildResponse(startTime, fastResp, err)
		fasthttp.ReleaseRequest(fastReq)
		fasthttp.ReleaseResponse(fastResp)
		return resp, err
	case <-ctx.Done():
		go func() {
			<-ch
			fasthttp.ReleaseRequest(fastReq)
			fasthttp.ReleaseResponse(fastResp)
		}()
		endTime := time.Now()
		return &Response{
			Stats: &ResponseStats{
				StartTime: startTime,
				EndTime:   endTime,
				TotalTime: endTime.Sub(startTime),
			},
		}, ctx.Err()
	}
}

// IsTimeout 判断是否为超时错误
func (fc *FastHTTPClient) IsTimeout(err error) bool {
	if errors.Is(err, fasthttp.ErrTimeout) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
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
