package http

import (
	"context"
	"testing"
	"time"
)

// MockClient 模拟 HTTP 客户端
type MockClient struct {
	response   *Response
	err        error
	callCount  int
	lastMethod string
	lastURL    string
}

func (m *MockClient) Do(req *Request) (*Response, error) {
	m.callCount++
	m.lastMethod = req.Method
	m.lastURL = req.URL
	return m.response, m.err
}

func (m *MockClient) IsTimeout(err error) bool {
	return false
}

// TestMiddlewareOrder 测试洋葱模型中间件的执行顺序
func TestMiddlewareOrder(t *testing.T) {
	var order []string

	mockClient := &MockClient{
		response: &Response{
			StatusCode: 200,
			Body:       []byte(`{"code": 0, "data": "test"}`),
			Stats:      &ResponseStats{},
		},
	}

	chain := newMiddlewareChain(mockClient)

	chain.use(func(ctx *Context) {
		order = append(order, "middleware1-before")
		ctx.Next()
		order = append(order, "middleware1-after")
	})

	chain.use(func(ctx *Context) {
		order = append(order, "middleware2-before")
		ctx.Next()
		order = append(order, "middleware2-after")
	})

	chain.use(func(ctx *Context) {
		order = append(order, "middleware3-before")
		ctx.Next()
		order = append(order, "middleware3-after")
	})

	ctx := &Context{
		Ctx: context.Background(),
		Request: &Request{
			Method: "GET",
			URL:    "https://example.com/test",
		},
	}

	chain.execute(ctx)

	expectedOrder := []string{
		"middleware1-before",
		"middleware2-before",
		"middleware3-before",
		"middleware3-after",
		"middleware2-after",
		"middleware1-after",
	}

	if len(order) != len(expectedOrder) {
		t.Errorf("expected %d steps, got %d", len(expectedOrder), len(order))
	}

	for i, step := range expectedOrder {
		if order[i] != step {
			t.Errorf("step %d: expected %s, got %s", i, step, order[i])
		}
	}

	t.Logf("Execution order: %v", order)
}

// TestMiddlewareAbort 测试中间件中断
func TestMiddlewareAbort(t *testing.T) {
	mockClient := &MockClient{
		response: &Response{
			StatusCode: 200,
			Body:       []byte(`{"code": 0}`),
			Stats:      &ResponseStats{},
		},
	}

	chain := newMiddlewareChain(mockClient)

	var executed []string

	chain.use(func(ctx *Context) {
		executed = append(executed, "middleware1-before")
		ctx.Next()
		executed = append(executed, "middleware1-after")
	})

	chain.use(func(ctx *Context) {
		executed = append(executed, "middleware2-abort")
		ctx.Abort() // 中断
	})

	chain.use(func(ctx *Context) {
		executed = append(executed, "middleware3-should-not-run")
		ctx.Next()
	})

	ctx := &Context{
		Ctx: context.Background(),
		Request: &Request{
			Method: "GET",
			URL:    "https://example.com/test",
		},
	}

	chain.execute(ctx)

	// 验证第三个中间件没有执行
	for _, step := range executed {
		if step == "middleware3-should-not-run" {
			t.Error("middleware3 should not have been executed")
		}
	}

	if !ctx.IsAborted() {
		t.Error("context should be aborted")
	}

	t.Logf("Executed: %v", executed)
}

// TestTimingMiddleware 测试计时中间件
func TestTimingMiddleware(t *testing.T) {
	mockClient := &MockClient{
		response: &Response{
			StatusCode: 200,
			Body:       []byte(`{"code": 0}`),
			Stats:      &ResponseStats{},
		},
	}

	chain := newMiddlewareChain(mockClient)
	chain.use(TimingMiddleware())

	chain.use(func(ctx *Context) {
		time.Sleep(10 * time.Millisecond)
		ctx.Next()
	})

	ctx := &Context{
		Ctx: context.Background(),
		Request: &Request{
			Method: "GET",
			URL:    "https://example.com/test",
		},
	}

	chain.execute(ctx)

	duration, ok := ctx.Get("request_duration")
	if !ok {
		t.Error("request_duration not set")
	}

	d, ok := duration.(time.Duration)
	if !ok {
		t.Error("request_duration is not time.Duration")
	}

	if d < 10*time.Millisecond {
		t.Errorf("duration too short: %v", d)
	}

	t.Logf("Request duration: %v", d)
}

// TestContextValues 测试上下文值传递
func TestContextValues(t *testing.T) {
	mockClient := &MockClient{
		response: &Response{
			StatusCode: 200,
			Body:       []byte(`{}`),
			Stats:      &ResponseStats{},
		},
	}

	chain := newMiddlewareChain(mockClient)

	chain.use(func(ctx *Context) {
		ctx.Set("user_id", "12345")
		ctx.Set("count", int64(100))
		ctx.Next()
	})

	chain.use(func(ctx *Context) {
		userID := ctx.GetString("user_id")
		if userID != "12345" {
			t.Errorf("expected user_id=12345, got %s", userID)
		}

		count := ctx.GetInt64("count")
		if count != 100 {
			t.Errorf("expected count=100, got %d", count)
		}

		ctx.Next()
	})

	ctx := &Context{
		Ctx: context.Background(),
		Request: &Request{
			Method: "GET",
			URL:    "https://example.com/test",
		},
	}

	chain.execute(ctx)
}

// TestComposeMiddleware 测试中间件组合
func TestComposeMiddleware(t *testing.T) {
	var order []string

	mockClient := &MockClient{
		response: &Response{
			StatusCode: 200,
			Body:       []byte(`{}`),
			Stats:      &ResponseStats{},
		},
	}

	composed := Compose(
		func(ctx *Context) {
			order = append(order, "composed1-before")
			ctx.Next()
			order = append(order, "composed1-after")
		},
		func(ctx *Context) {
			order = append(order, "composed2-before")
			ctx.Next()
			order = append(order, "composed2-after")
		},
	)

	chain := newMiddlewareChain(mockClient)
	chain.use(func(ctx *Context) {
		order = append(order, "outer-before")
		ctx.Next()
		order = append(order, "outer-after")
	})
	chain.use(composed)

	ctx := &Context{
		Ctx: context.Background(),
		Request: &Request{
			Method: "GET",
			URL:    "https://example.com/test",
		},
	}

	chain.execute(ctx)

	t.Logf("Execution order: %v", order)
}

// TestConditionalMiddleware 测试条件中间件
func TestConditionalMiddleware(t *testing.T) {
	mockClient := &MockClient{
		response: &Response{
			StatusCode: 200,
			Body:       []byte(`{}`),
			Stats:      &ResponseStats{},
		},
	}

	var executed []string

	chain := newMiddlewareChain(mockClient)

	chain.use(ConditionalMiddleware(
		func(ctx *Context) bool { return true },
		func(ctx *Context) {
			executed = append(executed, "should-run")
			ctx.Next()
		},
	))

	chain.use(ConditionalMiddleware(
		func(ctx *Context) bool { return false },
		func(ctx *Context) {
			executed = append(executed, "should-not-run")
			ctx.Next()
		},
	))

	ctx := &Context{
		Ctx: context.Background(),
		Request: &Request{
			Method: "GET",
			URL:    "https://example.com/test",
		},
	}

	chain.execute(ctx)

	if len(executed) != 1 || executed[0] != "should-run" {
		t.Errorf("unexpected executed: %v", executed)
	}
}

// TestRequestBuilder 测试请求构建器
func TestRequestBuilder(t *testing.T) {
	mockClient := &MockClient{
		response: &Response{
			StatusCode: 200,
			Body:       []byte(`{"code": 0, "data": {"name": "test"}}`),
			Stats:      &ResponseStats{},
		},
	}

	builder := NewRequestBuilder(mockClient)
	builder.NoKeepLog().NoSendErrorMsg()

	var middlewareExecuted bool
	builder.Use(func(ctx *Context) {
		middlewareExecuted = true
		ctx.Next()
	})

	result := builder.Execute(
		context.Background(),
		"GET",
		"https://example.com/test",
		nil,
		nil,
		nil,
	)

	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}

	if !middlewareExecuted {
		t.Error("middleware was not executed")
	}

	if mockClient.callCount != 1 {
		t.Errorf("expected 1 call, got %d", mockClient.callCount)
	}

	data, err := Parse[map[string]interface{}](result, "data")
	if err != nil {
		t.Errorf("parse error: %v", err)
	}

	if data["name"] != "test" {
		t.Errorf("expected name=test, got %v", data["name"])
	}
}

// TestRecoveryMiddleware 测试恢复中间件
func TestRecoveryMiddleware(t *testing.T) {
	mockClient := &MockClient{
		response: &Response{
			StatusCode: 200,
			Body:       []byte(`{}`),
			Stats:      &ResponseStats{},
		},
	}

	chain := newMiddlewareChain(mockClient)
	chain.use(RecoveryMiddleware())
	chain.use(func(ctx *Context) {
		panic("test panic")
	})

	ctx := &Context{
		Ctx: context.Background(),
		Request: &Request{
			Method: "GET",
			URL:    "https://example.com/test",
		},
	}

	chain.execute(ctx)

	if ctx.Error == nil {
		t.Error("expected error from panic recovery")
	}

	t.Logf("Recovered error: %v", ctx.Error)
}

// TestStatusCodeCheckMiddleware 测试状态码检查中间件
func TestStatusCodeCheckMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		expectedCodes []int
		expectError   bool
	}{
		{"200 OK with default check", 200, nil, false},
		{"404 with default check", 404, nil, true},
		{"200 with explicit codes", 200, []int{200, 201}, false},
		{"201 with explicit codes", 201, []int{200, 201}, false},
		{"404 with explicit codes", 404, []int{200, 201}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockClient{
				response: &Response{
					StatusCode: tt.statusCode,
					Body:       []byte(`{}`),
					Stats:      &ResponseStats{},
				},
			}

			chain := newMiddlewareChain(mockClient)
			chain.use(StatusCodeCheckMiddleware(tt.expectedCodes...))

			ctx := &Context{
				Ctx: context.Background(),
				Request: &Request{
					Method: "GET",
					URL:    "https://example.com/test",
				},
			}

			chain.execute(ctx)

			if tt.expectError && ctx.Error == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && ctx.Error != nil {
				t.Errorf("unexpected error: %v", ctx.Error)
			}
		})
	}
}

// TestCodeCheckMiddleware 测试业务状态码检查中间件
func TestCodeCheckMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		code        int64
		field       string
		expectError bool
	}{
		{"code 0 success", `{"code": 0}`, 0, "", false},
		{"code 0 fail", `{"code": 1}`, 0, "", true},
		{"custom field success", `{"status": 200}`, 200, "status", false},
		{"missing field", `{"data": "test"}`, 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockClient{
				response: &Response{
					StatusCode: 200,
					Body:       []byte(tt.body),
					Stats:      &ResponseStats{},
				},
			}

			chain := newMiddlewareChain(mockClient)
			if tt.field != "" {
				chain.use(CodeCheckMiddleware(tt.code, tt.field))
			} else {
				chain.use(CodeCheckMiddleware(tt.code))
			}

			ctx := &Context{
				Ctx: context.Background(),
				Request: &Request{
					Method: "GET",
					URL:    "https://example.com/test",
				},
			}

			chain.execute(ctx)

			if tt.expectError && ctx.Error == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && ctx.Error != nil {
				t.Errorf("unexpected error: %v", ctx.Error)
			}
		})
	}
}
