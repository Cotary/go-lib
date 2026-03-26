package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func Test_DefaultHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"name":"test","age":18}`))
	}))
	defer srv.Close()

	type TestUser struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	result := FastHTTP().Use(func(ctx *Context) {
		fmt.Println("before request")
		ctx.Next()
		fmt.Println("after request")
	}).Execute(context.Background(), "GET", srv.URL, nil, nil, nil)
	user, err := Parse[TestUser](result, "")
	assert.NoError(t, err)
	assert.Equal(t, "test", user.Name)
	assert.Equal(t, 18, user.Age)
}

// setupTestServer 启动一个带有延迟的测试服务器。
func setupTestServer(delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	}))
}

// TestHttpRequest_Timeout 验证在 server 延迟 5s 的场景下，
// 当我们设置超时时间为 2s，客户端会返回超时错误。
func TestHttpRequest_Timeout(t *testing.T) {
	srv := setupTestServer(5 * time.Second)
	defer srv.Close()

	// 测试 fasthttp 客户端
	t.Run("Fasthttp Timeout", func(t *testing.T) {
		fastClient := &FastHTTPClient{client: &fasthttp.Client{}}
		builder := NewRequestBuilder(fastClient).SetTimeout(2 * time.Second)

		res := builder.Execute(
			context.Background(),
			http.MethodGet,
			srv.URL,
			nil, nil, nil,
		)

		if res.Error == nil {
			t.Fatal("expected timeout error, but got nil")
		}
		if !fastClient.IsTimeout(res.Error) {
			t.Fatalf("expected timeout error, got: %v", res.Error)
		}
	})

	// 测试 resty 客户端
	t.Run("Resty Timeout", func(t *testing.T) {
		restyClient := &RestyClient{client: resty.New()}
		builder := NewRequestBuilder(restyClient).SetTimeout(2 * time.Second)

		res := builder.Execute(
			context.Background(),
			http.MethodGet,
			srv.URL,
			nil, nil, nil,
		)

		if res.Error == nil {
			t.Fatal("expected timeout error, but got nil")
		}
		if !restyClient.IsTimeout(res.Error) {
			t.Fatalf("expected timeout error, got: %v", res.Error)
		}
	})
}

type StatusResponse struct {
	Status string `json:"status"`
}

// TestHttpRequest_Success 验证在延迟 1s 的场景下，
// 设置超时 3s 可以正常返回并解析 JSON。
func TestHttpRequest_Success(t *testing.T) {
	srv := setupTestServer(1 * time.Second)
	defer srv.Close()

	// 测试 fasthttp 客户端
	t.Run("Fasthttp Success", func(t *testing.T) {
		fastClient := &FastHTTPClient{client: &fasthttp.Client{}}
		builder := NewRequestBuilder(fastClient).SetTimeout(3 * time.Second)

		res := builder.Execute(
			context.Background(),
			http.MethodGet,
			srv.URL,
			nil, nil, nil,
		)

		if res.Error != nil {
			t.Fatalf("expected no error, got: %v", res.Error)
		}

		data, err := Parse[StatusResponse](res, "")
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if data.Status != "ok" {
			t.Fatalf("expected status=ok, got: %q", data.Status)
		}
	})

	// 测试 resty 客户端
	t.Run("Resty Success", func(t *testing.T) {
		restyClient := &RestyClient{client: resty.New()}
		builder := NewRequestBuilder(restyClient).SetTimeout(3 * time.Second)

		res := builder.Execute(
			context.Background(),
			http.MethodGet,
			srv.URL,
			nil, nil, nil,
		)

		if res.Error != nil {
			t.Fatalf("expected no error, got: %v", res.Error)
		}

		data, err := Parse[StatusResponse](res, "")
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if data.Status != "ok" {
			t.Fatalf("expected status=ok, got: %q", data.Status)
		}
	})
}

func TestHttpRequest_Concurrent(t *testing.T) {
	const concurrency = 10

	t.Run("Fasthttp ConcurrentSuccess", func(t *testing.T) {
		srv := setupTestServer(100 * time.Millisecond)
		defer srv.Close()

		var wg sync.WaitGroup
		errs := make([]error, concurrency)
		fastClient := &FastHTTPClient{client: &fasthttp.Client{}}

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				builder := NewRequestBuilder(fastClient).SetTimeout(500 * time.Millisecond)

				res := builder.Execute(context.Background(), http.MethodGet, srv.URL, nil, nil, nil)
				errs[idx] = res.Error
				if res.Error == nil {
					d, parseErr := Parse[StatusResponse](res, "")
					if parseErr != nil {
						errs[idx] = parseErr
					} else if d.Status != "ok" {
						errs[idx] = errors.New("status != ok")
					}
				}
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Errorf("goroutine %d expected success, got error: %v", i, err)
			}
		}
	})

	t.Run("Resty ConcurrentSuccess", func(t *testing.T) {
		srv := setupTestServer(100 * time.Millisecond)
		defer srv.Close()

		var wg sync.WaitGroup
		errs := make([]error, concurrency)
		restyClient := &RestyClient{client: resty.New()}

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				builder := NewRequestBuilder(restyClient).SetTimeout(500 * time.Millisecond)

				res := builder.Execute(context.Background(), http.MethodGet, srv.URL, nil, nil, nil)
				errs[idx] = res.Error
				if res.Error == nil {
					d, parseErr := Parse[StatusResponse](res, "")
					if parseErr != nil {
						errs[idx] = parseErr
					} else if d.Status != "ok" {
						errs[idx] = errors.New("status != ok")
					}
				}
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Errorf("goroutine %d expected success, got error: %v", i, err)
			}
		}
	})

	t.Run("Fasthttp ConcurrentTimeout", func(t *testing.T) {
		srv := setupTestServer(300 * time.Millisecond)
		defer srv.Close()

		var wg sync.WaitGroup
		errs := make([]error, concurrency)
		fastClient := &FastHTTPClient{client: &fasthttp.Client{}}

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				builder := NewRequestBuilder(fastClient).SetTimeout(100 * time.Millisecond)

				res := builder.Execute(context.Background(), http.MethodGet, srv.URL, nil, nil, nil)
				errs[idx] = res.Error
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if err == nil {
				t.Errorf("goroutine %d expected timeout error, got nil", i)
				continue
			}
			if !fastClient.IsTimeout(err) {
				t.Errorf("goroutine %d expected timeout error, got %v", i, err)
			}
		}
	})

	t.Run("Resty ConcurrentTimeout", func(t *testing.T) {
		srv := setupTestServer(300 * time.Millisecond)
		defer srv.Close()

		var wg sync.WaitGroup
		errs := make([]error, concurrency)
		restyClient := &RestyClient{client: resty.New()}

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				builder := NewRequestBuilder(restyClient).SetTimeout(100 * time.Millisecond)

				res := builder.Execute(context.Background(), http.MethodGet, srv.URL, nil, nil, nil)
				errs[idx] = res.Error
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if err == nil {
				t.Errorf("goroutine %d expected timeout error, got nil", i)
				continue
			}
			if !restyClient.IsTimeout(err) {
				t.Errorf("goroutine %d expected timeout error, got %v", i, err)
			}
		}
	})
}

func TestResponseStats_Resty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	client := NewRestyClient()
	builder := NewRequestBuilder(client)

	result := builder.Execute(context.Background(), "GET", srv.URL, nil, nil, nil)

	assert.NoError(t, result.Error)
	assert.NotNil(t, result.Response)
	assert.NotNil(t, result.Response.Stats)

	stats := result.Response.Stats
	assert.True(t, stats.TotalTime > 0)
	assert.True(t, stats.StartTime.Before(stats.EndTime))

	t.Logf("Total Time: %v", stats.TotalTime)
}

func TestResponseStats_FastHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	client := NewFastHTTPClient()
	builder := NewRequestBuilder(client)

	result := builder.Execute(context.Background(), "GET", srv.URL, nil, nil, nil)

	assert.NoError(t, result.Error)
	assert.NotNil(t, result.Response)
	assert.NotNil(t, result.Response.Stats)

	stats := result.Response.Stats
	assert.True(t, stats.TotalTime >= 0)
	assert.False(t, stats.StartTime.After(stats.EndTime))

	t.Logf("Total Time: %v", stats.TotalTime)
}

// TestRetryMiddleware_NoSideEffects 验证 RetryMiddleware 重试时不会重复执行前置中间件
func TestRetryMiddleware_NoSideEffects(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"server error"}`))
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	authCallCount := 0
	client := NewFastHTTPClient()
	builder := NewRequestBuilder(client).NoKeepLog().NoSendErrorMsg()
	builder.Use(
		func(ctx *Context) {
			authCallCount++
			if ctx.Request.Headers == nil {
				ctx.Request.Headers = make(map[string]string)
			}
			ctx.Request.Headers["X-Auth"] = "token"
			ctx.Next()
		},
		RetryMiddleware(3, 10*time.Millisecond),
	)

	result := builder.Execute(context.Background(), "GET", srv.URL, nil, nil, nil)

	assert.NoError(t, result.Error)
	assert.Equal(t, 200, result.Response.StatusCode)
	// 前置中间件只执行一次，HTTP 请求执行了 3 次（2 次 500 + 1 次 200）
	assert.Equal(t, 1, authCallCount, "auth middleware should only run once")
	assert.Equal(t, 3, callCount, "server should receive 3 requests")
}

// TestRetryMiddleware_4xxNoRetry 验证 4xx 错误不重试
func TestRetryMiddleware_4xxNoRetry(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	client := NewFastHTTPClient()
	builder := NewRequestBuilder(client).NoKeepLog().NoSendErrorMsg()
	builder.Use(RetryMiddleware(3, 10*time.Millisecond))

	result := builder.Execute(context.Background(), "GET", srv.URL, nil, nil, nil)

	assert.Equal(t, 400, result.Response.StatusCode)
	assert.Equal(t, 1, callCount, "4xx should not trigger retry")
}

// TestParseTo_EdgeCases 验证 ParseTo 的边界情况
func TestParseTo_EdgeCases(t *testing.T) {
	t.Run("nil dest", func(t *testing.T) {
		mockClient := &MockClient{
			response: &Response{
				StatusCode: 200,
				Body:       []byte(`{"code": 0, "data": "test"}`),
				Stats:      &ResponseStats{},
			},
		}
		builder := NewRequestBuilder(mockClient).NoKeepLog().NoSendErrorMsg()
		result := builder.Execute(context.Background(), "GET", "https://example.com", nil, nil, nil)
		err := result.ParseTo("data", nil)
		assert.NoError(t, err)
	})

	t.Run("empty body", func(t *testing.T) {
		mockClient := &MockClient{
			response: &Response{
				StatusCode: 200,
				Body:       []byte{},
				Stats:      &ResponseStats{},
			},
		}
		builder := NewRequestBuilder(mockClient).NoKeepLog().NoSendErrorMsg()
		result := builder.Execute(context.Background(), "GET", "https://example.com", nil, nil, nil)
		var dest string
		err := result.ParseTo("", &dest)
		assert.Error(t, err)
	})

	t.Run("non-json body", func(t *testing.T) {
		mockClient := &MockClient{
			response: &Response{
				StatusCode: 200,
				Body:       []byte(`not json`),
				Stats:      &ResponseStats{},
			},
		}
		builder := NewRequestBuilder(mockClient).NoKeepLog().NoSendErrorMsg()
		result := builder.Execute(context.Background(), "GET", "https://example.com", nil, nil, nil)
		var dest map[string]interface{}
		err := result.ParseTo("", &dest)
		assert.Error(t, err)
	})

	t.Run("path not found", func(t *testing.T) {
		mockClient := &MockClient{
			response: &Response{
				StatusCode: 200,
				Body:       []byte(`{"code": 0}`),
				Stats:      &ResponseStats{},
			},
		}
		builder := NewRequestBuilder(mockClient).NoKeepLog().NoSendErrorMsg()
		result := builder.Execute(context.Background(), "GET", "https://example.com", nil, nil, nil)
		var dest string
		err := result.ParseTo("data.user.name", &dest)
		assert.Error(t, err)
	})

	t.Run("non-2xx status", func(t *testing.T) {
		mockClient := &MockClient{
			response: &Response{
				StatusCode: 500,
				Body:       []byte(`{"error":"internal"}`),
				Stats:      &ResponseStats{},
			},
		}
		builder := NewRequestBuilder(mockClient).NoKeepLog().NoSendErrorMsg()
		result := builder.Execute(context.Background(), "GET", "https://example.com", nil, nil, nil)
		var dest map[string]interface{}
		err := result.ParseTo("", &dest)
		assert.Error(t, err)
	})
}

// TestFastHTTPClient_ContextCancel 验证 FastHTTP 客户端响应 context 取消
func TestFastHTTPClient_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	fastClient := NewFastHTTPClient()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	builder := NewRequestBuilder(fastClient).NoKeepLog().NoSendErrorMsg()
	result := builder.Execute(ctx, "GET", srv.URL, nil, nil, nil)
	elapsed := time.Since(start)

	assert.Error(t, result.Error)
	assert.True(t, fastClient.IsTimeout(result.Error),
		"expected timeout/context error, got: %v", result.Error)
	assert.Less(t, elapsed, 1*time.Second, "should return quickly on context cancel")
}

// TestBuildLogMap 验证 BuildLogMap 输出包含预期字段
func TestBuildLogMap(t *testing.T) {
	mockClient := &MockClient{
		response: &Response{
			StatusCode: 200,
			Body:       []byte(`{"code": 0}`),
			Stats:      &ResponseStats{TotalTime: 100 * time.Millisecond},
		},
	}

	builder := NewRequestBuilder(mockClient).NoKeepLog().NoSendErrorMsg()
	builder.Use(TimingMiddleware())

	result := builder.Execute(context.Background(), "GET", "https://example.com/test",
		map[string][]string{"q": {"1"}},
		map[string]string{"key": "value"},
		map[string]string{"Authorization": "Bearer xxx"},
	)

	logMap := result.BuildLogMap()
	assert.Equal(t, "https://example.com/test", logMap["Request URL"])
	assert.Equal(t, "GET", logMap["Request Method"])
	assert.NotNil(t, logMap["Response Status Code"])
	assert.NotNil(t, logMap["Duration"])
}
