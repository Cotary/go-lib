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
		builder := NewRequestBuilder[any](fastClient).SetTimeout(2 * time.Second)

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
		builder := NewRequestBuilder[any](restyClient).SetTimeout(2 * time.Second)

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
		builder := NewRequestBuilder[StatusResponse](fastClient).SetTimeout(3 * time.Second)

		res := builder.Execute(
			context.Background(),
			http.MethodGet,
			srv.URL,
			nil, nil, nil,
		)

		if res.Error != nil {
			t.Fatalf("expected no error, got: %v", res.Error)
		}

		data, err := res.Parse("")
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
		builder := NewRequestBuilder[StatusResponse](restyClient).SetTimeout(3 * time.Second)

		res := builder.Execute(
			context.Background(),
			http.MethodGet,
			srv.URL,
			nil, nil, nil,
		)

		if res.Error != nil {
			t.Fatalf("expected no error, got: %v", res.Error)
		}

		data, err := res.Parse("")
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
				builder := NewRequestBuilder[StatusResponse](fastClient).SetTimeout(500 * time.Millisecond)

				res := builder.Execute(context.Background(), http.MethodGet, srv.URL, nil, nil, nil)
				errs[idx] = res.Error
				if res.Error == nil {
					d, parseErr := res.Parse("")
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
				builder := NewRequestBuilder[StatusResponse](restyClient).SetTimeout(500 * time.Millisecond)

				res := builder.Execute(context.Background(), http.MethodGet, srv.URL, nil, nil, nil)
				errs[idx] = res.Error
				if res.Error == nil {
					d, parseErr := res.Parse("")
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
				builder := NewRequestBuilder[any](fastClient).SetTimeout(100 * time.Millisecond)

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
				builder := NewRequestBuilder[any](restyClient).SetTimeout(100 * time.Millisecond)

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
	client := NewRestyClient()
	builder := NewRequestBuilder[any](client)

	ctx := context.Background()
	result := builder.Execute(ctx, "GET", "https://httpbin.org/get", nil, nil, nil)

	assert.NoError(t, result.Error)
	assert.NotNil(t, result.Response)
	assert.NotNil(t, result.Response.Stats)

	stats := result.Response.Stats
	assert.True(t, stats.TotalTime > 0)
	assert.True(t, stats.StartTime.Before(stats.EndTime))

	t.Logf("Total Time: %v", stats.TotalTime)
}

func TestResponseStats_FastHTTP(t *testing.T) {
	client := NewFastHTTPClient()
	builder := NewRequestBuilder[any](client)

	ctx := context.Background()
	result := builder.Execute(ctx, "GET", "https://httpbin.org/get", nil, nil, nil)

	assert.NoError(t, result.Error)
	assert.NotNil(t, result.Response)
	assert.NotNil(t, result.Response.Stats)

	stats := result.Response.Stats
	assert.True(t, stats.TotalTime > 0)
	assert.True(t, stats.StartTime.Before(stats.EndTime))

	t.Logf("Total Time: %v", stats.TotalTime)
}

func Test_DefaultHTTP(t *testing.T) {
	type TestUser struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	user, err := fastHTTP[TestUser]().Execute(context.Background(), "GET", "https://httpbin.org/get", nil, nil, nil).Parse("")
	fmt.Println(user, err)
}
