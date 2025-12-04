package http

import (
	"context"
	"math"
)

// ============================================================================
// 中间件上下文
// ============================================================================

const abortIndex int8 = math.MaxInt8 >> 1 // 中断标记

// Context 中间件上下文，用于在中间件链中传递数据
//
// 类似 Gin 的 Context 设计，支持：
//   - Next() 调用下一个中间件
//   - Abort() 中断中间件链
//   - AddError() 添加错误
type Context struct {
	Ctx      context.Context        // Go 原生上下文
	Request  *Request               // HTTP 请求
	Response *Response              // HTTP 响应
	Error    error                  // 主要错误
	Errors   []error                // 错误列表
	values   map[string]interface{} // 中间件间传递的数据

	// 中间件链控制
	handlers []Middleware // 中间件列表
	index    int8         // 当前执行索引
}

// Next 执行下一个中间件
//
// 用法:
//
//	func MyMiddleware() Middleware {
//	    return func(ctx *Context) {
//	        // 前置处理
//	        ctx.Next()
//	        // 后置处理
//	    }
//	}
func (c *Context) Next() {
	c.index++
	for c.index < int8(len(c.handlers)) {
		c.handlers[c.index](c)
		c.index++
	}
}

// Abort 中断中间件链，后续中间件不会执行
func (c *Context) Abort() {
	c.index = abortIndex
}

// AbortWithError 中断并设置错误
func (c *Context) AbortWithError(err error) {
	c.AddError(err)
	c.Abort()
}

// IsAborted 检查是否已中断
func (c *Context) IsAborted() bool {
	return c.index >= abortIndex
}

// AddError 添加错误到错误列表，并设置主错误
func (c *Context) AddError(err error) {
	if err == nil {
		return
	}
	c.Errors = append(c.Errors, err)
	if c.Error == nil {
		c.Error = err
	}
}

// Set 设置上下文值
func (c *Context) Set(key string, value interface{}) {
	if c.values == nil {
		c.values = make(map[string]interface{})
	}
	c.values[key] = value
}

// Get 获取上下文值
func (c *Context) Get(key string) (interface{}, bool) {
	if c.values == nil {
		return nil, false
	}
	v, ok := c.values[key]
	return v, ok
}

// GetString 获取字符串类型的上下文值
func (c *Context) GetString(key string) string {
	if v, ok := c.Get(key); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetInt64 获取 int64 类型的上下文值
func (c *Context) GetInt64(key string) int64 {
	if v, ok := c.Get(key); ok {
		if i, ok := v.(int64); ok {
			return i
		}
	}
	return 0
}

// ============================================================================
// 中间件类型定义
// ============================================================================

// Middleware 中间件类型（Gin 风格）
//
// 中间件可以在调用 ctx.Next() 前后分别执行逻辑：
//   - ctx.Next() 之前：请求前置处理
//   - ctx.Next() 之后：响应后置处理
//
// 示例:
//
//	func MyMiddleware() Middleware {
//	    return func(ctx *Context) {
//	        log.Println("请求开始")
//	        ctx.Next()
//	        log.Println("请求结束")
//	    }
//	}
type Middleware func(ctx *Context)

// ============================================================================
// 中间件工具函数
// ============================================================================

// Compose 组合多个中间件为一个
func Compose(middlewares ...Middleware) Middleware {
	return func(ctx *Context) {
		// 保存原始状态
		originalHandlers := ctx.handlers
		originalIndex := ctx.index

		// 设置组合的中间件
		ctx.handlers = middlewares
		ctx.index = -1
		ctx.Next()

		// 恢复状态
		ctx.handlers = originalHandlers
		ctx.index = originalIndex
	}
}

// ConditionalMiddleware 条件中间件，根据条件决定是否执行
func ConditionalMiddleware(condition func(ctx *Context) bool, middleware Middleware) Middleware {
	return func(ctx *Context) {
		if condition(ctx) {
			middleware(ctx)
		} else {
			ctx.Next()
		}
	}
}
