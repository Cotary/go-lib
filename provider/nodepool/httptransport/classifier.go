package httptransport

import (
	"context"
	"net/http"

	"github.com/Cotary/go-lib/provider/nodepool"
)

// ClassifyFunc 自定义分类函数签名。
// 返回值决定该响应对节点健康度的影响。
type ClassifyFunc func(statusCode int, body []byte, err error) nodepool.NodeStatus

// Classifier 基于 HTTP 状态码的响应分类器，实现 nodepool.ResponseClassifier。
//
// 默认分类规则：
//   - NodeStatusSuccess: 2xx / 3xx
//   - NodeStatusFail: 5xx、429（限流）、连接错误、超时
//   - NodeStatusBizError: 其他 4xx（400/401/403/404 等）
//
// 可通过 ClassifierOption 自定义哪些状态码视为故障或业务错误，
// 也可通过 WithCustomClassify 完全覆盖默认行为。
type Classifier struct {
	failCodes      map[int]bool
	bizErrCodes    map[int]bool
	customClassify ClassifyFunc
}

// ClassifierOption 是 Classifier 的 Functional Option 类型。
type ClassifierOption func(*Classifier)

// NewClassifier 创建基于 HTTP 状态码的响应分类器。
//
// 使用示例:
//
//	// 使用默认规则
//	classifier := httptransport.NewClassifier()
//
//	// 自定义：将 403 也视为节点故障（如 IP 封禁场景）
//	classifier := httptransport.NewClassifier(
//	    httptransport.WithFailCodes(http.StatusForbidden),
//	)
func NewClassifier(opts ...ClassifierOption) *Classifier {
	c := &Classifier{
		failCodes:   map[int]bool{http.StatusTooManyRequests: true},
		bizErrCodes: make(map[int]bool),
	}
	for _, fn := range opts {
		fn(c)
	}
	return c
}

// WithFailCodes 将指定状态码标记为节点故障（NodeStatusFail）。
// 节点故障会触发重试和健康度扣分。
// 默认已包含 429（Too Many Requests）。
func WithFailCodes(codes ...int) ClassifierOption {
	return func(c *Classifier) {
		for _, code := range codes {
			c.failCodes[code] = true
			delete(c.bizErrCodes, code)
		}
	}
}

// WithBizErrCodes 将指定状态码标记为业务错误（NodeStatusBizError）。
// 业务错误不会触发重试，也不影响节点健康度。
func WithBizErrCodes(codes ...int) ClassifierOption {
	return func(c *Classifier) {
		for _, code := range codes {
			c.bizErrCodes[code] = true
			delete(c.failCodes, code)
		}
	}
}

// WithCustomClassify 设置自定义分类函数，完全覆盖默认的状态码分类逻辑。
// 适用于需要根据响应体内容（如业务码）判断请求结果的场景。
func WithCustomClassify(fn ClassifyFunc) ClassifierOption {
	return func(c *Classifier) {
		c.customClassify = fn
	}
}

// Classify 实现 nodepool.ResponseClassifier 接口。
//
// 分类优先级：
//  1. 存在传输错误 → NodeStatusFail
//  2. 自定义分类函数（如已设置）
//  3. 按 failCodes / bizErrCodes 映射表判断
//  4. 兜底：5xx → Fail，4xx → BizError，其他 → Success
func (c *Classifier) Classify(_ context.Context, _ string, resp *nodepool.Response, err error) nodepool.NodeStatus {
	if err != nil && resp == nil {
		return nodepool.NodeStatusFail
	}

	var statusCode int
	var body []byte
	if resp != nil {
		if httpResp, ok := resp.Data.(*HTTPResponse); ok && httpResp != nil {
			statusCode = httpResp.StatusCode
			body = httpResp.Body
		}
	}

	if err != nil {
		if c.customClassify != nil {
			return c.customClassify(statusCode, body, err)
		}
		return nodepool.NodeStatusFail
	}

	if c.customClassify != nil {
		return c.customClassify(statusCode, body, nil)
	}

	if c.failCodes[statusCode] {
		return nodepool.NodeStatusFail
	}
	if c.bizErrCodes[statusCode] {
		return nodepool.NodeStatusBizError
	}

	if statusCode >= 500 {
		return nodepool.NodeStatusFail
	}
	if statusCode >= 400 {
		return nodepool.NodeStatusBizError
	}

	return nodepool.NodeStatusSuccess
}
