package http

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/log"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
)

// 有效的HTTP方法
var validHTTPMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodPost:    true,
	http.MethodPut:     true,
	http.MethodDelete:  true,
	http.MethodPatch:   true,
	http.MethodHead:    true,
	http.MethodOptions: true,
}

// isValidHTTPMethod 验证HTTP方法是否有效
func isValidHTTPMethod(method string) bool {
	return validHTTPMethods[method]
}

// Request defines the payload for an HTTP request.
type Request struct {
	Ctx     context.Context
	Method  string
	URL     string
	Query   map[string][]string
	Body    interface{}
	Headers map[string]string
	Timeout time.Duration
}

// Response holds all information from an HTTP response.
type Response struct {
	StatusCode int
	Header     map[string][]string
	Body       []byte
	// 统计数据
	Stats *ResponseStats
}

func (r *Response) String() string {
	if len(r.Body) == 0 {
		return ""
	}
	return strings.TrimSpace(string(r.Body))
}

// ResponseStats holds statistics about the HTTP response
type ResponseStats struct {
	// 执行时间统计
	TotalTime time.Duration // 总执行时间
	// 时间戳
	StartTime time.Time // 请求开始时间
	EndTime   time.Time // 请求结束时间
}

// Result encapsulates the outcome of an HTTP request.
type Result struct {
	Request      *Request
	Response     *Response
	Error        error
	Handlers     []ResponseHandler
	KeepLog      bool
	SendErrorMsg bool
}

// IClient is the core interface for executing HTTP requests.
// It defines the contract for different HTTP client implementations (e.g., fasthttp, net/http).
type IClient interface {
	Do(request *Request) (*Response, error)
	IsTimeout(err error) bool
}

// RequestBuilder is a generic struct for configuring and sending HTTP requests.
// T is a type that must implement the IClient interface.
type RequestBuilder[T IClient] struct {
	client       T
	handlers     []RequestHandler
	keepLog      bool
	sendErrorMsg bool
	timeout      time.Duration
}

// NewRequestBuilder creates a new RequestBuilder with a specific IClient implementation.
func NewRequestBuilder[T IClient](client T) *RequestBuilder[T] {
	return &RequestBuilder[T]{
		client:       client,
		keepLog:      true,
		sendErrorMsg: true,
	}
}

func (rb *RequestBuilder[T]) NoSendErrorMsg() *RequestBuilder[T] {
	rb.sendErrorMsg = false
	return rb
}

func (rb *RequestBuilder[T]) NoKeepLog() *RequestBuilder[T] {
	rb.keepLog = false
	return rb
}

func (rb *RequestBuilder[T]) SetHandlers(handler ...RequestHandler) *RequestBuilder[T] {
	rb.handlers = handler
	return rb
}

func (rb *RequestBuilder[T]) SetTimeout(timeout time.Duration) *RequestBuilder[T] {
	rb.timeout = timeout
	return rb
}

// Execute performs the HTTP request and returns a Result.
func (rb *RequestBuilder[T]) Execute(
	ctx context.Context,
	method, url string,
	query map[string][]string,
	body interface{},
	headers map[string]string,
) *Result {
	if ctx == nil {
		ctx = context.Background()
	}

	req := &Request{
		Ctx:     ctx,
		Method:  method,
		URL:     url,
		Query:   query,
		Body:    body,
		Headers: headers,
		Timeout: rb.timeout,
	}

	res := &Result{
		Request:      req,
		KeepLog:      rb.keepLog,
		SendErrorMsg: rb.sendErrorMsg,
	}

	// 参数校验
	switch {
	case url == "":
		res.Error = errors.New("URL cannot be empty")
		return res
	case !isValidHTTPMethod(method):
		res.Error = fmt.Errorf("invalid HTTP method: %s", method)
		return res
	}

	// 执行 handler
	for _, handler := range rb.handlers {
		if err := handler(req); err != nil {
			res.Error = err
			return res
		}
	}

	// 发起请求
	res.Response, res.Error = rb.client.Do(req)

	if res.KeepLog {
		res.Log(log.WithContext(ctx))
	}

	return res
}

func (r *Result) SetHandlers(handlers ...ResponseHandler) *Result {
	r.Handlers = handlers
	return r
}

func (r *Result) Log(logEntry log.Logger) *Result {
	ctx := r.Request.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	logMap := r.getLogMap(ctx)
	if r.Error != nil {
		if logEntry != nil {
			logEntry.WithContext(ctx).WithFields(logMap).Error("HTTP Request")
		}
		if r.SendErrorMsg {
			e.SendMessage(ctx, errors.New("HTTP Request Error:"+utils.Json(logMap)))
		}
	} else {
		if logEntry != nil {
			logEntry.WithContext(ctx).WithFields(logMap).Info("HTTP Request")
		}
	}
	return r
}

func (r *Result) Parse(path string, data interface{}) error {
	ctx := r.Request.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	if r.Error != nil {
		return r.Error
	}
	logMap := r.getLogMap(ctx)
	if r.Response.StatusCode < 200 || r.Response.StatusCode >= 300 {
		return errors.New("response status code error: " + utils.Json(logMap))
	}

	isJson := utils.IsJson(r.Response.Body)
	if !isJson {
		return errors.New("response is not json: " + utils.Json(logMap))
	}

	// Execute response handlers before parsing
	for _, f := range r.Handlers {
		if err := f(r); err != nil {
			return errors.Wrap(err, utils.Json(logMap))
		}
	}

	if data == nil {
		return nil
	}

	// Handle parsing with gjson
	var respJson string
	if path != "" {
		gj := gjson.ParseBytes(r.Response.Body)
		value := gj.Get(path)
		if !value.Exists() {
			return errors.New(fmt.Sprintf("path not found: %s , response: %s", path, utils.Json(logMap)))
		}
		respJson = value.String()
	} else {
		respJson = string(r.Response.Body)
	}

	if err := utils.StringTo(respJson, data); err != nil {
		return e.Err(err, fmt.Sprintf("response parse error, response: %s", utils.Json(logMap)))
	}
	return nil
}

func (r *Result) getLogMap(ctx context.Context) map[string]interface{} {
	logMap := map[string]interface{}{
		"Context ID": ctx.Value(defined.RequestID),
	}

	if r.Request != nil {
		logMap["Request URL"] = r.Request.URL
		logMap["Request Method"] = r.Request.Method
		logMap["Request Headers"] = r.Request.Headers
		logMap["Request Query"] = r.Request.Query
		if r.Request.Body != nil {
			logMap["Request Body"] = r.Request.Body
		}
	}

	if r.Response != nil {
		logMap["Response Status Code"] = r.Response.StatusCode
		logMap["Response Headers"] = r.Response.Header
		logMap["Response Body"] = string(r.Response.Body)

		// 添加统计信息到日志
		if r.Response.Stats != nil {
			logMap["Total Time"] = r.Response.Stats.TotalTime.String()
		}
	}

	if r.Error != nil {
		logMap["Request Error"] = r.Error.Error()
	}
	return logMap
}
