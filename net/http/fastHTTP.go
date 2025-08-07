package http

import (
	"time"

	"github.com/Cotary/go-lib/common/utils"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
)

var DefaultFastHTTPClient = NewFastHTTPClient()

type FastHTTPClient struct {
	client *fasthttp.Client
}

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

func (fc *FastHTTPClient) Do(req *Request) (*Response, error) {
	fastReq := fasthttp.AcquireRequest()
	fastResp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(fastReq)
	defer fasthttp.ReleaseResponse(fastResp)

	// 验证请求参数
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

	// 设置超时时间
	if req.Timeout > 0 {
		fastReq.SetTimeout(req.Timeout)
	}

	err := fc.client.Do(fastReq, fastResp)

	// 拷贝响应数据到通用结构体
	resp := &Response{}
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

func (fc *FastHTTPClient) IsTimeout(err error) bool {
	return errors.Is(err, fasthttp.ErrTimeout)
}
