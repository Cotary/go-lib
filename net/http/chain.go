package http

import (
	"github.com/pkg/errors"
)

// ============================================================================
// 中间件链
// ============================================================================

// middlewareChain 中间件链，负责按洋葱模型执行中间件
type middlewareChain struct {
	middlewares []Middleware
	client      IClient
}

// newMiddlewareChain 创建新的中间件链
func newMiddlewareChain(client IClient) *middlewareChain {
	return &middlewareChain{
		client:      client,
		middlewares: make([]Middleware, 0),
	}
}

// use 添加中间件
func (c *middlewareChain) use(middlewares ...Middleware) {
	c.middlewares = append(c.middlewares, middlewares...)
}

// execute 执行中间件链
//
// 执行顺序（洋葱模型）：
//
//	请求 → [中间件1前] → [中间件2前] → HTTP请求 → [中间件2后] → [中间件1后] → 响应
func (c *middlewareChain) execute(ctx *Context) {
	// 添加实际请求作为最后一个"中间件"
	handlers := make([]Middleware, len(c.middlewares)+1)
	copy(handlers, c.middlewares)
	handlers[len(handlers)-1] = c.doRequestMiddleware()

	// 设置中间件链到上下文
	ctx.handlers = handlers
	ctx.index = -1
	ctx.client = c.client

	// 开始执行
	ctx.Next()
}

// doRequestMiddleware 返回执行实际 HTTP 请求的中间件
func (c *middlewareChain) doRequestMiddleware() Middleware {
	return func(ctx *Context) {
		if c.client == nil {
			ctx.AddError(errors.New("HTTP client is nil"))
			return
		}

		resp, err := c.client.Do(ctx.Request)
		ctx.Response = resp
		if err != nil {
			ctx.AddError(err)
		}
	}
}
