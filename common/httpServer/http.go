package httpServer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/log"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"net/http"
)

type HttpClient struct {
	*resty.Client
	BeforeHandler []BeforeHandler
}

func NewHttpClient() *HttpClient {
	restyClient := resty.New()
	return &HttpClient{
		Client: restyClient,
	}
}

func (hClient *HttpClient) SetBeforeHandler(handler []BeforeHandler) *HttpClient {
	hClient.BeforeHandler = handler
	return hClient
}

func (hClient *HttpClient) HttpRequest(ctx context.Context, method string, url string, query map[string][]string, body interface{}, headers map[string]string) *RestyResult {
	for _, handler := range hClient.BeforeHandler {
		if err := handler(ctx, &method, &url, query, body, headers); err != nil {
			return &RestyResult{
				Context: ctx,
				Error:   err,
			}
		}
	}

	req := hClient.Client.R()
	if query != nil {
		req.SetQueryParamsFromValues(query)
	}
	if headers != nil {
		req.SetHeaders(headers)
	}
	if body != nil {
		req.SetBody(body)
	}

	resp, err := executeRequest(req, method, url)
	rr := &RestyResult{
		Context:  ctx,
		Client:   hClient.Client,
		Response: resp,
		Error:    err,
	}
	rr.Log(log.DefaultLogger)
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

type RestyResult struct {
	context.Context
	*resty.Client
	*resty.Response
	Logs  map[string]interface{}
	Error error
}

func (t *RestyResult) Log(logEntry *logrus.Logger) *RestyResult {
	ctx := t.Context
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
			logEntry.WithContext(ctx).WithFields(logMap).Error()
		}
		e.SendMessage(ctx, errors.New("HTTP Request Error:"+utils.Json(logMap)))
	} else {
		if logEntry != nil {
			logEntry.WithContext(ctx).WithFields(logMap).Info()
		}
	}
	t.Logs = logMap
	return t
}

// Parse 解析响应
func (t *RestyResult) Parse(checkFuncList []CheckFunc, path string, data interface{}) error {
	if t.Error != nil {
		return t.Error
	}

	errMsg := fmt.Sprintf("\nResponse not success: %s\n", utils.Json(t.Logs))
	gj := gjson.Parse(t.String())
	if gj.Type != gjson.JSON {
		return errors.New("Response is not json")
	}
	if !t.IsSuccess() {
		return errors.New(errMsg)
	}

	for _, f := range checkFuncList {
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

	return json.Unmarshal([]byte(respJson), data)
}
