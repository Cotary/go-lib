package http

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
)

// ============================================================================
// 日志配置
// ============================================================================

// logConfig 日志配置，用于在中间件上下文中传递
type logConfig struct {
	keepLog      bool
	sendErrorMsg bool
}

// logConfigKey 日志配置在 Context 中的键
const logConfigKey = "__log_config__"

// ============================================================================
// 请求构建器
// ============================================================================

// RequestBuilder 泛型请求构建器
//
// 使用洋葱模型中间件处理请求和响应，T 表示期望的响应类型
type RequestBuilder[T any] struct {
	client       IClient
	chain        *middlewareChain
	keepLog      bool
	sendErrorMsg bool
	timeout      time.Duration
}

// NewRequestBuilder 创建泛型请求构建器
func NewRequestBuilder[T any](client IClient) *RequestBuilder[T] {
	return &RequestBuilder[T]{
		client:       client,
		chain:        newMiddlewareChain(client),
		keepLog:      true,
		sendErrorMsg: true,
	}
}

// Use 添加中间件（支持链式调用）
//
// 中间件按添加顺序执行，形成洋葱模型：
//
//	builder.Use(
//	    RecoveryMiddleware(),        // 最外层
//	    LoggingMiddleware(logger),   // 第二层
//	    AuthMiddleware("app", "secret"), // 最内层
//	)
func (rb *RequestBuilder[T]) Use(middlewares ...Middleware) *RequestBuilder[T] {
	rb.chain.use(middlewares...)
	return rb
}

// NoSendErrorMsg 禁用错误消息发送
func (rb *RequestBuilder[T]) NoSendErrorMsg() *RequestBuilder[T] {
	rb.sendErrorMsg = false
	return rb
}

// NoKeepLog 禁用日志记录
func (rb *RequestBuilder[T]) NoKeepLog() *RequestBuilder[T] {
	rb.keepLog = false
	return rb
}

// SetTimeout 设置请求超时时间
func (rb *RequestBuilder[T]) SetTimeout(timeout time.Duration) *RequestBuilder[T] {
	rb.timeout = timeout
	return rb
}

// Execute 执行 HTTP 请求
//
// 参数:
//   - ctx: Go 上下文
//   - method: HTTP 方法（GET、POST 等）
//   - url: 请求 URL
//   - query: 查询参数
//   - body: 请求体
//   - headers: 请求头
//
// 返回:
//   - *Result[T]: 包含响应数据和错误信息的结果对象
func (rb *RequestBuilder[T]) Execute(
	goCtx context.Context,
	method, url string,
	query map[string][]string,
	body interface{},
	headers map[string]string,
) *Result[T] {
	if goCtx == nil {
		goCtx = context.Background()
	}

	req := &Request{
		Ctx:     goCtx,
		Method:  method,
		URL:     url,
		Query:   query,
		Body:    body,
		Headers: headers,
		Timeout: rb.timeout,
	}

	// 创建中间件上下文
	ctx := &Context{
		Ctx:     goCtx,
		Request: req,
		values:  make(map[string]interface{}),
	}

	// 存储日志配置到上下文，供日志中间件使用
	ctx.Set(logConfigKey, &logConfig{
		keepLog:      rb.keepLog,
		sendErrorMsg: rb.sendErrorMsg,
	})

	res := &Result[T]{
		Context:      ctx,
		keepLog:      rb.keepLog,
		sendErrorMsg: rb.sendErrorMsg,
	}

	// 参数校验
	switch {
	case url == "":
		ctx.Error = errors.New("URL cannot be empty")
		return res
	case !isValidHTTPMethod(method):
		ctx.Error = fmt.Errorf("invalid HTTP method: %s", method)
		return res
	}

	// 构建完整的中间件链：日志中间件 + 用户中间件
	chain := newMiddlewareChain(rb.client)
	// 日志中间件放在最外层，确保能记录完整的请求/响应
	chain.use(RequestLogMiddleware())
	chain.use(rb.chain.middlewares...)

	// 执行中间件链
	chain.execute(ctx)

	return res
}
