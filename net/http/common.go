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

// IClient is the core interface for executing HTTP requests.
// It defines the contract for different HTTP client implementations (e.g., fasthttp, net/http).
type IClient interface {
	Do(request *Request) (*Response, error)
	IsTimeout(err error) bool
}

// ============================================================================
// BaseResult 基础结果（非泛型）
// ============================================================================

// BaseResult 基础结果结构体，包含所有非泛型字段
type BaseResult struct {
	Request      *Request
	Response     *Response
	Error        error
	Handlers     []ResponseHandler
	KeepLog      bool
	SendErrorMsg bool
}

func (r *BaseResult) SetHandlers(handlers ...ResponseHandler) *BaseResult {
	r.Handlers = handlers
	return r
}

func (r *BaseResult) Log(logEntry log.Logger) *BaseResult {
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

func (r *BaseResult) getLogMap(ctx context.Context) map[string]interface{} {
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

		if r.Response.Stats != nil {
			logMap["Total Time"] = r.Response.Stats.TotalTime.String()
		}
	}

	if r.Error != nil {
		logMap["Request Error"] = r.Error.Error()
	}
	return logMap
}

// ============================================================================
// RequestBuilder[T] 泛型请求构建器
// ============================================================================

// RequestBuilder 泛型请求构建器，T 表示期望的响应类型
type RequestBuilder[T any] struct {
	client       IClient
	handlers     []RequestHandler
	keepLog      bool
	sendErrorMsg bool
	timeout      time.Duration
}

// NewRequestBuilder 创建泛型请求构建器
func NewRequestBuilder[T any](client IClient) *RequestBuilder[T] {
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

// Execute 执行 HTTP 请求，返回 Result[T]
func (rb *RequestBuilder[T]) Execute(
	ctx context.Context,
	method, url string,
	query map[string][]string,
	body interface{},
	headers map[string]string,
) *Result[T] {
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

	res := &Result[T]{
		BaseResult: BaseResult{
			Request:      req,
			KeepLog:      rb.keepLog,
			SendErrorMsg: rb.sendErrorMsg,
		},
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

// ============================================================================
// Result[T] 泛型结果（嵌入 BaseResult）
// ============================================================================

// Result 泛型版本的结果，T 表示期望解析的响应类型
type Result[T any] struct {
	BaseResult
}

// SetHandlers 设置响应处理器（返回 Result[T] 以支持链式调用）
func (r *Result[T]) SetHandlers(handlers ...ResponseHandler) *Result[T] {
	r.Handlers = handlers
	return r
}

// Log 记录日志（返回 Result[T] 以支持链式调用）
func (r *Result[T]) Log(logEntry log.Logger) *Result[T] {
	r.BaseResult.Log(logEntry)
	return r
}

// Parse 解析响应到泛型类型 T
func (r *Result[T]) Parse(path string) (T, error) {
	var zero T
	ctx := r.Request.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	if r.Error != nil {
		return zero, r.Error
	}
	if r.Response == nil {
		return zero, errors.New("response is nil")
	}

	logMap := r.getLogMap(ctx)
	if r.Response.StatusCode < 200 || r.Response.StatusCode >= 300 {
		return zero, errors.New("response status code error: " + utils.Json(logMap))
	}

	isJson := utils.IsJson(r.Response.Body)
	if !isJson {
		return zero, errors.New("response is not json: " + utils.Json(logMap))
	}

	// Execute response handlers before parsing
	for _, f := range r.Handlers {
		if err := f(&r.BaseResult); err != nil {
			return zero, errors.Wrap(err, utils.Json(logMap))
		}
	}

	// Handle parsing with gjson
	var respJson string
	if path != "" {
		gj := gjson.ParseBytes(r.Response.Body)
		value := gj.Get(path)
		if !value.Exists() {
			return zero, errors.New(fmt.Sprintf("path not found: %s , response: %s", path, utils.Json(logMap)))
		}
		respJson = value.String()
	} else {
		respJson = string(r.Response.Body)
	}

	// 使用泛型转换
	data, err := utils.AnyToAny[T](respJson)
	if err != nil {
		return zero, e.Err(err, fmt.Sprintf("response parse error, response: %s", utils.Json(logMap)))
	}
	return data, nil
}

// MustParse 解析响应，失败时 panic
func (r *Result[T]) MustParse(path string) T {
	data, err := r.Parse(path)
	if err != nil {
		panic(err)
	}
	return data
}
