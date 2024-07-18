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
	"github.com/tidwall/gjson"
	"net/http"
)

type HttpClient struct {
	*resty.Client
	*resty.Request
}

func NewHttpClient() *HttpClient {
	restyClient := resty.New()
	newHttp := &HttpClient{
		Client:  restyClient,
		Request: restyClient.R(),
	}
	newHttp.Client.SetRetryCount(2)
	return newHttp
}

type checkFunc func(res *resty.Response, gj gjson.Result) error

var DefaultCheck []checkFunc = []checkFunc{HttpStatusCheckFunc}

func (hClient HttpClient) HttpRequest(ctx context.Context, method string, url string, query map[string][]string, body interface{}, headers map[string]string) RestyResult {
	if query != nil {
		hClient.Request.SetQueryParamsFromValues(query)
	}
	if headers != nil {
		hClient.Request.SetHeaders(headers)
	}
	if body != nil {
		hClient.Request.SetBody(body)
	}

	var resp *resty.Response
	var err error
	switch method {
	case http.MethodGet:
		resp, err = hClient.Request.Get(url)
	case http.MethodPost:
		resp, err = hClient.Request.Post(url)
	case http.MethodPut:
		resp, err = hClient.Request.Put(url)
	case http.MethodDelete:
		resp, err = hClient.Request.Delete(url)
	default:
		resp, err = hClient.Request.Get(url)
	}

	logMap := map[string]interface{}{
		"Context ID":           ctx.Value(defined.RequestID),
		"Request URL":          url,
		"Request Method":       method,
		"Request Headers":      headers,
		"Request Query":        query,
		"Request Body":         body,
		"Response Status Code": resp.StatusCode(),
		"Response Headers":     resp.Header(),
		"Response Body":        resp.String(),
	}
	if err != nil {
		logMap["Request Error"] = err.Error()
		log.WithContext(ctx).WithFields(logMap).Error()

		//发送报警
		e.SendMessage(ctx, errors.New("HTTP Request Error:"+utils.Json(logMap)))
	} else {
		log.WithContext(ctx).WithFields(logMap).Info()
	}

	return RestyResult{
		Client:   hClient.Client,
		Response: resp,
		Error:    err,
	}
}

type RestyResult struct {
	*resty.Client
	*resty.Response
	Error error
}

// Parse 解析响应
func (t RestyResult) Parse(checkFuncList []checkFunc, path string, data interface{}) error {
	if t.Error != nil {
		return t.Error
	}

	var respJson string
	gj := gjson.Parse(t.String())

	for _, f := range checkFuncList {
		err := f(t.Response, gj)
		if err != nil {
			return err
		}
	}

	if path != "" {
		value := gj.Get(path)
		if !value.Exists() {
			return fmt.Errorf("path not found: %s", path)
		}
		respJson = value.String()
	} else {
		respJson = gj.String()
	}

	return json.Unmarshal([]byte(respJson), data)
}

// HttpStatusCheckFunc http状态校验
var HttpStatusCheckFunc = func(res *resty.Response, gj gjson.Result) error {
	if !res.IsSuccess() || gj.Type != gjson.JSON {
		return errors.New("HTTP Response Fail:" + res.Request.URL + " " + res.Status() + " " + res.String())
	}
	return nil
}

// CodeZeroCheckFunc 适用于老框架
var CodeZeroCheckFunc = func(res *resty.Response, gj gjson.Result) error {
	if gj.Get("code").Int() != 0 {
		errMsg := gj.Get("data").String()
		return errors.New("response err:" + errMsg)
	}
	return nil
}

// CodeTwoHundredCheckFunc 适用于新的go框架
var CodeTwoHundredCheckFunc = func(res *resty.Response, gj gjson.Result) error {
	if gj.Get("code").Int() != 200 {
		errMsg := gj.Get("message").String()
		return errors.New("response err:" + errMsg)
	}
	return nil
}
