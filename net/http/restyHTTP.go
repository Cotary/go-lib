package http

import (
	"context"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	"net"
	"net/http"
)

var DefaultRestyClient = NewRestyClient()

type RestyClient struct {
	client *resty.Client
}

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

func (rc *RestyClient) Do(req *Request) (*Response, error) {

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

	var restyResp *resty.Response
	var err error

	switch req.Method {
	case http.MethodGet:
		restyResp, err = restyReq.Get(req.URL)
	case http.MethodPost:
		restyResp, err = restyReq.Post(req.URL)
	case http.MethodPut:
		restyResp, err = restyReq.Put(req.URL)
	case http.MethodDelete:
		restyResp, err = restyReq.Delete(req.URL)
	default:
		restyResp, err = restyReq.Get(req.URL)
	}

	resp := &Response{}
	if restyResp != nil {
		resp.StatusCode = restyResp.StatusCode()
		resp.Body = restyResp.Body()
		resp.Header = restyResp.Header()
	}

	return resp, err
}

func (rc *RestyClient) IsTimeout(err error) bool {
	// resty 内部的超时错误是 net.timeout 错误，需要进行类型断言
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}
