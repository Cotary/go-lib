package httpServer

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/log"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"net/http"
	"time"
)

var defaultClient = resty.New().SetTransport(&http.Transport{
	MaxIdleConns:        10000,
	MaxIdleConnsPerHost: 300,
})

func Client() *resty.Client {
	return defaultClient
}

func Request() *RestyRequest {
	return &RestyRequest{
		Request:      defaultClient.R(),
		keepLog:      true,
		sendErrorMsg: true,
	}
}

type RestyRequest struct {
	*resty.Request

	Handlers     []RequestHandler
	timeout      time.Duration
	keepLog      bool
	sendErrorMsg bool
}

func NewRequest(client *resty.Client) *RestyRequest {
	return &RestyRequest{
		Request: client.R(),
	}
}

func (request *RestyRequest) NoSendErrorMsg() *RestyRequest {
	request.sendErrorMsg = false
	return request
}
func (request *RestyRequest) NoKeepLog() *RestyRequest {
	request.keepLog = false
	return request
}
func (request *RestyRequest) SetHandlers(handler ...RequestHandler) *RestyRequest {
	request.Handlers = handler
	return request
}
func (request *RestyRequest) SetTimeout(timeout time.Duration) *RestyRequest {
	request.timeout = timeout
	return request
}

func (request *RestyRequest) HttpRequest(ctx context.Context, method string, url string, query map[string][]string, body interface{}, headers map[string]string) (res *RestyResult) {
	res = &RestyResult{
		RestyRequest: request,
	}
	if query == nil {
		query = map[string][]string{}
	}
	if headers == nil {
		headers = map[string]string{}
	}
	for _, handler := range request.Handlers {
		if err := handler(ctx, &method, &url, query, body, headers); err != nil {
			res.Error = err
			return res
		}
	}

	request.SetContext(ctx)
	if request.timeout > 0 {
		newCtx, cancel := context.WithTimeout(ctx, request.timeout)
		defer cancel()
		request.SetContext(newCtx)
	}

	request.SetQueryParamsFromValues(query)
	request.SetHeaders(headers)
	if body != nil {
		request.SetBody(body)
	}

	resp, err := executeRequest(request.Request, method, url)
	rr := &RestyResult{
		Response: resp,
		Error:    err,
	}
	if request.keepLog {
		rr.Log(log.WithContext(ctx))
	}
	return rr
}

func executeRequest(req *resty.Request, method, url string) (*resty.Response, error) {
	switch method {
	case http.MethodGet:
		return req.Get(url)
	case http.MethodPost:
		return req.Post(url)
	case http.MethodPut:
		return req.Put(url)
	case http.MethodDelete:
		return req.Delete(url)
	default:
		return req.Get(url)
	}
}

// RestyResult Response Result
type RestyResult struct {
	*RestyRequest
	*resty.Response
	Logs     map[string]interface{}
	Handlers []ResponseHandler
	Error    error
}

func (t *RestyResult) SetHandlers(handlers ...ResponseHandler) *RestyResult {
	t.Handlers = handlers
	return t
}

func (t *RestyResult) Log(logEntry log.Logger) *RestyResult {
	ctx := t.Context()
	logMap := map[string]interface{}{
		"Context ID": ctx.Value(defined.RequestID),
	}

	if t.Response != nil {
		logMap["Request URL"] = t.Response.Request.URL
		logMap["Request Method"] = t.Response.Request.Method
		logMap["Request Headers"] = t.Response.Request.Header
		logMap["Request Query"] = t.Response.Request.RawRequest.URL.Query()
		logMap["Request Body"] = t.Response.Request.Body
		logMap["Response Status Code"] = t.Response.StatusCode()
		logMap["Response Headers"] = t.Response.Header()
		logMap["Response Body"] = t.Response.String()
	}

	if t.Error != nil {
		logMap["Request Error"] = t.Error.Error()
		if logEntry != nil {
			logEntry.WithContext(ctx).WithFields(logMap).Error("HTTP Request")
		}
		if t.sendErrorMsg {
			e.SendMessage(ctx, errors.New("HTTP Request Error:"+utils.Json(logMap)))
		}

	} else {
		if logEntry != nil {
			logEntry.WithContext(ctx).WithFields(logMap).Info("HTTP Request")
		}
	}
	t.Logs = logMap
	return t
}

// Parse 解析响应
func (t *RestyResult) Parse(path string, data interface{}) error {
	if t.Error != nil {
		return t.Error
	}

	logTxt := utils.Json(t.Logs)
	if t.Logs == nil {
		logTxt = t.Response.String()
	}

	errMsg := fmt.Sprintf("Response not success: %s", logTxt)
	if !t.IsSuccess() {
		return errors.New(errMsg)
	}

	isJson := utils.IsJson(t.String())
	if !isJson {
		return errors.New("Response is not json: " + errMsg)
	}
	gj := gjson.Parse(t.String())

	for _, f := range t.Handlers {
		if err := f(t, gj); err != nil {
			return errors.Wrap(err, errMsg)
		}
	}

	if data == nil {
		return nil
	}

	var respJson string
	if path != "" {
		value := gj.Get(path)
		if !value.Exists() {
			return errors.New(fmt.Sprintf("path not found: %s", path))
		}
		respJson = value.String()
	} else {
		respJson = gj.String()
	}

	err := utils.StringTo(respJson, data)
	if err != nil {
		return e.Err(err, "response parse error")
	}
	return nil
}
