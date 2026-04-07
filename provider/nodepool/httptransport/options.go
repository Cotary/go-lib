package httptransport

import (
	"time"

	nethttp "github.com/Cotary/go-lib/net/http"
)

// Options 配置 HTTP Transport 的行为。
type Options struct {
	client       nethttp.IClient
	headers      map[string]string
	nodeHeaders  map[string]map[string]string
	middlewares  []nethttp.Middleware
	keepLog      bool
	sendErrorMsg bool
	timeout      time.Duration
}

func defaultOptions() Options {
	return Options{
		client:       nethttp.DefaultFastHTTPClient,
		headers:      make(map[string]string),
		nodeHeaders:  make(map[string]map[string]string),
		keepLog:      true,
		sendErrorMsg: true,
	}
}

// Option 是 Transport 的 Functional Option 类型。
type Option func(*Options)

// WithClient 设置自定义的 HTTP 客户端实现。
// 默认使用 FastHTTPClient，可替换为 Resty 或其他实现了 IClient 的客户端。
func WithClient(client nethttp.IClient) Option {
	return func(o *Options) {
		if client != nil {
			o.client = client
		}
	}
}

// WithDefaultHeaders 设置全局默认请求头。
// 这些 Header 会被节点级和请求级 Header 覆盖。
func WithDefaultHeaders(headers map[string]string) Option {
	return func(o *Options) {
		if headers != nil {
			o.headers = headers
		}
	}
}

// WithNodeHeaders 设置指定节点（endpoint）的专用请求头。
// 优先级高于全局默认 Header，低于请求级 Header。
// 可多次调用为不同节点设置不同的 Header。
func WithNodeHeaders(endpoint string, headers map[string]string) Option {
	return func(o *Options) {
		if endpoint != "" && headers != nil {
			o.nodeHeaders[endpoint] = headers
		}
	}
}

// WithMiddleware 添加自定义中间件。
// 中间件按添加顺序执行，遵循洋葱模型，可用于认证、签名、自定义日志等。
//
// 示例:
//
//	httptransport.New(
//	    httptransport.WithMiddleware(
//	        nethttp.AuthMiddleware("appId", "secret", "md5"),
//	        nethttp.TimingMiddleware(),
//	    ),
//	)
func WithMiddleware(middlewares ...nethttp.Middleware) Option {
	return func(o *Options) {
		o.middlewares = append(o.middlewares, middlewares...)
	}
}

// WithKeepLog 控制是否记录请求日志。
// 默认为 true，设置为 false 可关闭内置的请求日志记录。
func WithKeepLog(keep bool) Option {
	return func(o *Options) {
		o.keepLog = keep
	}
}

// WithSendErrorMsg 控制请求出错时是否发送错误通知。
// 默认为 true，设置为 false 可关闭错误消息推送。
func WithSendErrorMsg(send bool) Option {
	return func(o *Options) {
		o.sendErrorMsg = send
	}
}

// WithTimeout 设置默认请求超时时间。
// 如果 HTTPRequest 未指定超时，则使用此默认值。
// 零值表示不设置超时（依赖底层客户端的默认超时）。
func WithTimeout(d time.Duration) Option {
	return func(o *Options) {
		if d >= 0 {
			o.timeout = d
		}
	}
}
