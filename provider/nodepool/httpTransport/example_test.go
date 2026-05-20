package httpTransport_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	nethttp "github.com/Cotary/go-lib/net/http"
	"github.com/Cotary/go-lib/provider/nodepool"
	"github.com/Cotary/go-lib/provider/nodepool/httpTransport"
)

// mockHTTPClient 用于示例的 mock HTTP 客户端
type mockHTTPClient struct{}

func (m *mockHTTPClient) Do(req *nethttp.Request) (*nethttp.Response, error) {
	return &nethttp.Response{
		StatusCode: 200,
		Body:       []byte(fmt.Sprintf(`{"url":"%s","method":"%s"}`, req.URL, req.Method)),
		Header:     map[string][]string{"Content-Type": {"application/json"}},
		Stats:      &nethttp.ResponseStats{TotalTime: 10 * time.Millisecond},
	}, nil
}

func (m *mockHTTPClient) IsTimeout(err error) bool { return false }

// Example_basic 演示最基本的使用方式：创建 Transport + Classifier，配合 nodepool 使用。
func Example_basic() {
	// 创建 HTTP Transport，使用 mock 客户端模拟真实请求
	transport := httpTransport.New(
		httpTransport.WithClient(&mockHTTPClient{}),
		httpTransport.WithDefaultHeaders(map[string]string{
			"Content-Type": "application/json",
		}),
		httpTransport.WithKeepLog(false),
	)

	// 创建基于 HTTP 状态码的分类器
	classifier := httpTransport.NewClassifier()

	// 创建节点池
	pool, err := nodepool.New(transport, classifier, []nodepool.NodeConfig{
		{Endpoint: "https://api1.example.com"},
		{Endpoint: "https://api2.example.com"},
	})
	if err != nil {
		fmt.Println("创建节点池失败:", err)
		return
	}
	defer pool.Close()

	// 发起 POST 请求
	resp, err := pool.Do(context.Background(), &nodepool.Request{
		Data: &httpTransport.HTTPRequest{
			Method: http.MethodPost,
			Path:   "/v1/chat/completions",
			Body:   map[string]any{"model": "gpt-4", "prompt": "hello"},
		},
	})
	if err != nil {
		fmt.Println("请求失败:", err)
		return
	}

	// 获取 HTTP 响应
	httpResp := resp.Data.(*httpTransport.HTTPResponse)
	fmt.Println("状态码:", httpResp.StatusCode)

	// Output:
	// 状态码: 200
}

// Example_withNodeHeaders 演示为不同节点设置不同的认证 Header。
func Example_withNodeHeaders() {
	transport := httpTransport.New(
		httpTransport.WithClient(&mockHTTPClient{}),
		httpTransport.WithDefaultHeaders(map[string]string{
			"Content-Type": "application/json",
		}),
		// 不同节点使用不同的 API Key
		httpTransport.WithNodeHeaders("https://api1.example.com", map[string]string{
			"Authorization": "Bearer key-for-api1",
		}),
		httpTransport.WithNodeHeaders("https://api2.example.com", map[string]string{
			"Authorization": "Bearer key-for-api2",
		}),
		httpTransport.WithKeepLog(false),
	)

	classifier := httpTransport.NewClassifier()

	pool, err := nodepool.New(transport, classifier, []nodepool.NodeConfig{
		{Endpoint: "https://api1.example.com"},
		{Endpoint: "https://api2.example.com"},
	})
	if err != nil {
		fmt.Println("创建节点池失败:", err)
		return
	}
	defer pool.Close()

	resp, err := pool.Do(context.Background(), &nodepool.Request{
		Data: &httpTransport.HTTPRequest{
			Method: http.MethodGet,
			Path:   "/v1/models",
		},
	})
	if err != nil {
		fmt.Println("请求失败:", err)
		return
	}

	httpResp := resp.Data.(*httpTransport.HTTPResponse)
	fmt.Println("状态码:", httpResp.StatusCode)

	// Output:
	// 状态码: 200
}

// Example_customClassifier 演示自定义分类器：根据响应体中的业务码判断请求结果。
func Example_customClassifier() {
	transport := httpTransport.New(
		httpTransport.WithClient(&mockHTTPClient{}),
		httpTransport.WithKeepLog(false),
	)

	// 根据业务码分类：HTTP 200 但业务码非 0 时视为业务错误
	classifier := httpTransport.NewClassifier(
		httpTransport.WithCustomClassify(func(statusCode int, body []byte, err error) nodepool.NodeStatus {
			if err != nil {
				return nodepool.NodeStatusFail
			}
			if statusCode >= 500 {
				return nodepool.NodeStatusFail
			}
			if statusCode >= 400 {
				return nodepool.NodeStatusBizError
			}
			return nodepool.NodeStatusSuccess
		}),
	)

	pool, err := nodepool.New(transport, classifier, []nodepool.NodeConfig{
		{Endpoint: "https://api.example.com"},
	})
	if err != nil {
		fmt.Println("创建节点池失败:", err)
		return
	}
	defer pool.Close()

	resp, err := pool.Do(context.Background(), &nodepool.Request{
		Data: &httpTransport.HTTPRequest{
			Path: "/health",
		},
	})
	if err != nil {
		fmt.Println("请求失败:", err)
		return
	}

	httpResp := resp.Data.(*httpTransport.HTTPResponse)
	fmt.Println("状态码:", httpResp.StatusCode)

	// Output:
	// 状态码: 200
}

// Example_withMiddleware 演示添加自定义中间件（认证签名、计时等）。
func Example_withMiddleware() {
	transport := httpTransport.New(
		httpTransport.WithClient(&mockHTTPClient{}),
		httpTransport.WithMiddleware(
			// 计时中间件
			nethttp.TimingMiddleware(),
			// 追踪中间件（自动注入 X-Request-ID）
			nethttp.TracingMiddleware(),
		),
		httpTransport.WithTimeout(10*time.Second),
		httpTransport.WithKeepLog(false),
	)

	classifier := httpTransport.NewClassifier()

	pool, err := nodepool.New(transport, classifier, []nodepool.NodeConfig{
		{Endpoint: "https://api.example.com"},
	})
	if err != nil {
		fmt.Println("创建节点池失败:", err)
		return
	}
	defer pool.Close()

	resp, err := pool.Do(context.Background(), &nodepool.Request{
		Data: &httpTransport.HTTPRequest{
			Method: http.MethodGet,
			Path:   "/v1/status",
		},
	})
	if err != nil {
		fmt.Println("请求失败:", err)
		return
	}

	httpResp := resp.Data.(*httpTransport.HTTPResponse)
	fmt.Println("状态码:", httpResp.StatusCode)

	// Output:
	// 状态码: 200
}
