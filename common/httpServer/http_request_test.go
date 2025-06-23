// http_request_test.go
package httpServer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestHttpRequest_Timeout 验证在 server 延迟 5s 的场景下，
// 当我们设置超时时间为 2s，HttpRequest 会返回 context.DeadlineExceeded。
func TestHttpRequest_Timeout(t *testing.T) {
	// 启动一个 5s 后才回应的测试服务器
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	// 构造带 2s 超时的请求
	ctx := context.Background()
	req := Request().SetTimeout(2 * time.Second)

	// 发起请求
	res := req.HttpRequest(ctx, http.MethodGet, srv.URL, nil, nil, nil)

	// 应该报超时错误
	if res.Error == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(res.Error, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", res.Error)
	}
}

// TestHttpRequest_Success 演示在延迟 1s 的场景下，
// 设置超时 3s 可以正常返回并解析 JSON。
func TestHttpRequest_Success(t *testing.T) {
	// 启动一个 1s 后回应的测试服务器
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	// 构造带 3s 超时的请求
	ctx := context.Background()
	req := Request().SetTimeout(3 * time.Second)

	// 发起请求
	res := req.HttpRequest(ctx, http.MethodGet, srv.URL, nil, nil, nil)
	if res.Error != nil {
		t.Fatalf("expected no error, got %v", res.Error)
	}

	// 解析响应体中的 "status" 字段
	var data struct{ Status string }
	if err := res.Parse("", &data); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if data.Status != "ok" {
		t.Fatalf("expected status=ok, got %q", data.Status)
	}
}

func TestHttpRequest_Concurrent(t *testing.T) {
	const concurrency = 10

	t.Run("ConcurrentSuccess", func(t *testing.T) {
		// 服务器延迟 100ms，超时设为 500ms，应全部成功
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(200)
			w.Write([]byte(`{"status":"ok"}`))
		}))
		defer srv.Close()

		var wg sync.WaitGroup
		errs := make([]error, concurrency)
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				req := Request().SetTimeout(500 * time.Millisecond)
				res := req.HttpRequest(context.Background(), http.MethodGet, srv.URL, nil, nil, nil)
				// 返回结果放到 errs[idx]
				errs[idx] = res.Error
				if res.Error == nil {
					// 额外校验一下 Parse 能拿到正确字段
					var d struct{ Status string }
					if parseErr := res.Parse("", &d); parseErr != nil {
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

	t.Run("ConcurrentTimeout", func(t *testing.T) {
		// 服务器延迟 300ms，超时设为 100ms，应全部超时
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
			w.WriteHeader(200)
			w.Write([]byte(`{"status":"ok"}`))
		}))
		defer srv.Close()

		var wg sync.WaitGroup
		errs := make([]error, concurrency)
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				req := Request().SetTimeout(100 * time.Millisecond)
				res := req.HttpRequest(context.Background(), http.MethodGet, srv.URL, nil, nil, nil)
				errs[idx] = res.Error
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if err == nil {
				t.Errorf("goroutine %d expected timeout error, got nil", i)
				continue
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("goroutine %d expected DeadlineExceeded, got %v", i, err)
			}
		}
	})
}
