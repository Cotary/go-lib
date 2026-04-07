package httptransport

import (
	"context"
	"net/http"
	"testing"
	"time"

	nethttp "github.com/Cotary/go-lib/net/http"
	"github.com/Cotary/go-lib/provider/nodepool"
)

// mockClient 用于测试的 mock HTTP 客户端
type mockClient struct {
	doFunc func(req *nethttp.Request) (*nethttp.Response, error)
}

func (m *mockClient) Do(req *nethttp.Request) (*nethttp.Response, error) {
	return m.doFunc(req)
}

func (m *mockClient) IsTimeout(err error) bool {
	return false
}

// =============================================================================
// buildURL 测试
// =============================================================================

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		path     string
		want     string
	}{
		{
			name:     "正常拼接",
			endpoint: "https://api.example.com",
			path:     "/v1/users",
			want:     "https://api.example.com/v1/users",
		},
		{
			name:     "endpoint 末尾有斜杠",
			endpoint: "https://api.example.com/",
			path:     "/v1/users",
			want:     "https://api.example.com/v1/users",
		},
		{
			name:     "path 无前导斜杠",
			endpoint: "https://api.example.com",
			path:     "v1/users",
			want:     "https://api.example.com/v1/users",
		},
		{
			name:     "path 为完整 URL",
			endpoint: "https://api.example.com",
			path:     "https://other.example.com/v2/data",
			want:     "https://other.example.com/v2/data",
		},
		{
			name:     "path 为空",
			endpoint: "https://api.example.com/v1",
			path:     "",
			want:     "https://api.example.com/v1",
		},
		{
			name:     "双斜杠清理",
			endpoint: "https://api.example.com/",
			path:     "/",
			want:     "https://api.example.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildURL(tt.endpoint, tt.path)
			if got != tt.want {
				t.Errorf("buildURL(%q, %q) = %q, want %q", tt.endpoint, tt.path, got, tt.want)
			}
		})
	}
}

// =============================================================================
// mergeHeaders 测试
// =============================================================================

func TestMergeHeaders(t *testing.T) {
	transport := New(
		WithDefaultHeaders(map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer default",
			"X-Global":      "global-value",
		}),
		WithNodeHeaders("https://node1.example.com", map[string]string{
			"Authorization": "Bearer node1",
			"X-Node":        "node1-value",
		}),
	)

	reqHeaders := map[string]string{
		"Authorization": "Bearer request",
		"X-Request":     "request-value",
	}

	merged := transport.mergeHeaders("https://node1.example.com", reqHeaders)

	expectations := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer request",
		"X-Global":      "global-value",
		"X-Node":        "node1-value",
		"X-Request":     "request-value",
	}

	for key, want := range expectations {
		if got := merged[key]; got != want {
			t.Errorf("Header[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestMergeHeaders_NoNodeHeaders(t *testing.T) {
	transport := New(
		WithDefaultHeaders(map[string]string{
			"Content-Type": "application/json",
		}),
	)

	merged := transport.mergeHeaders("https://unknown.example.com", map[string]string{
		"X-Custom": "value",
	})

	if got := merged["Content-Type"]; got != "application/json" {
		t.Errorf("Header[Content-Type] = %q, want %q", got, "application/json")
	}
	if got := merged["X-Custom"]; got != "value" {
		t.Errorf("Header[X-Custom] = %q, want %q", got, "value")
	}
}

// =============================================================================
// Transport.Execute 测试
// =============================================================================

func TestTransport_Execute_Success(t *testing.T) {
	mock := &mockClient{
		doFunc: func(req *nethttp.Request) (*nethttp.Response, error) {
			if req.URL != "https://api.example.com/v1/data" {
				t.Errorf("URL = %q, want %q", req.URL, "https://api.example.com/v1/data")
			}
			if req.Method != http.MethodPost {
				t.Errorf("Method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.Headers["Content-Type"] != "application/json" {
				t.Errorf("Content-Type = %q, want %q", req.Headers["Content-Type"], "application/json")
			}
			return &nethttp.Response{
				StatusCode: 200,
				Body:       []byte(`{"result":"ok"}`),
				Header:     map[string][]string{"X-Resp": {"value"}},
				Stats:      &nethttp.ResponseStats{TotalTime: 50 * time.Millisecond},
			}, nil
		},
	}

	transport := New(
		WithClient(mock),
		WithDefaultHeaders(map[string]string{"Content-Type": "application/json"}),
		WithKeepLog(false),
	)

	resp, err := transport.Execute(context.Background(), "https://api.example.com", &nodepool.Request{
		Data: &HTTPRequest{
			Method: http.MethodPost,
			Path:   "/v1/data",
			Body:   map[string]any{"key": "value"},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	httpResp, ok := resp.Data.(*HTTPResponse)
	if !ok {
		t.Fatalf("resp.Data type = %T, want *HTTPResponse", resp.Data)
	}
	if httpResp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", httpResp.StatusCode)
	}
	if string(httpResp.Body) != `{"result":"ok"}` {
		t.Errorf("Body = %q, want %q", string(httpResp.Body), `{"result":"ok"}`)
	}
}

func TestTransport_Execute_InvalidData(t *testing.T) {
	transport := New(WithKeepLog(false))

	_, err := transport.Execute(context.Background(), "https://api.example.com", &nodepool.Request{
		Data: "not an HTTPRequest",
	})
	if err == nil {
		t.Fatal("Execute() expected error for invalid Data type")
	}
}

func TestTransport_Execute_DefaultMethod(t *testing.T) {
	mock := &mockClient{
		doFunc: func(req *nethttp.Request) (*nethttp.Response, error) {
			if req.Method != http.MethodGet {
				t.Errorf("Method = %q, want %q (default)", req.Method, http.MethodGet)
			}
			return &nethttp.Response{
				StatusCode: 200,
				Body:       []byte("ok"),
				Stats:      &nethttp.ResponseStats{},
			}, nil
		},
	}

	transport := New(WithClient(mock), WithKeepLog(false))

	_, err := transport.Execute(context.Background(), "https://api.example.com", &nodepool.Request{
		Data: &HTTPRequest{Path: "/health"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

// =============================================================================
// HTTPRequest.GetMethod 测试
// =============================================================================

func TestHTTPRequest_GetMethod(t *testing.T) {
	tests := []struct {
		method string
		want   string
	}{
		{"", http.MethodGet},
		{http.MethodPost, http.MethodPost},
		{http.MethodPut, http.MethodPut},
	}

	for _, tt := range tests {
		req := &HTTPRequest{Method: tt.method}
		if got := req.GetMethod(); got != tt.want {
			t.Errorf("GetMethod() with Method=%q = %q, want %q", tt.method, got, tt.want)
		}
	}
}

// =============================================================================
// Classifier 测试
// =============================================================================

func TestClassifier_DefaultRules(t *testing.T) {
	c := NewClassifier()
	ctx := context.Background()

	tests := []struct {
		name       string
		statusCode int
		err        error
		want       nodepool.NodeStatus
	}{
		{"200 成功", 200, nil, nodepool.NodeStatusSuccess},
		{"201 成功", 201, nil, nodepool.NodeStatusSuccess},
		{"301 重定向成功", 301, nil, nodepool.NodeStatusSuccess},
		{"400 业务错误", 400, nil, nodepool.NodeStatusBizError},
		{"401 业务错误", 401, nil, nodepool.NodeStatusBizError},
		{"404 业务错误", 404, nil, nodepool.NodeStatusBizError},
		{"429 节点故障", 429, nil, nodepool.NodeStatusFail},
		{"500 节点故障", 500, nil, nodepool.NodeStatusFail},
		{"502 节点故障", 502, nil, nodepool.NodeStatusFail},
		{"503 节点故障", 503, nil, nodepool.NodeStatusFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &nodepool.Response{
				Data: &HTTPResponse{StatusCode: tt.statusCode},
			}
			got := c.Classify(ctx, "https://node1.example.com", resp, tt.err)
			if got != tt.want {
				t.Errorf("Classify(status=%d) = %d, want %d", tt.statusCode, got, tt.want)
			}
		})
	}
}

func TestClassifier_TransportError(t *testing.T) {
	c := NewClassifier()
	ctx := context.Background()

	got := c.Classify(ctx, "https://node1.example.com", nil, context.DeadlineExceeded)
	if got != nodepool.NodeStatusFail {
		t.Errorf("Classify() with transport error = %d, want NodeStatusFail", got)
	}
}

func TestClassifier_CustomFailCodes(t *testing.T) {
	c := NewClassifier(
		WithFailCodes(http.StatusForbidden),
	)
	ctx := context.Background()

	resp := &nodepool.Response{
		Data: &HTTPResponse{StatusCode: http.StatusForbidden},
	}
	got := c.Classify(ctx, "", resp, nil)
	if got != nodepool.NodeStatusFail {
		t.Errorf("Classify(403 as fail) = %d, want NodeStatusFail", got)
	}
}

func TestClassifier_CustomBizErrCodes(t *testing.T) {
	c := NewClassifier(
		WithBizErrCodes(http.StatusTooManyRequests),
	)
	ctx := context.Background()

	resp := &nodepool.Response{
		Data: &HTTPResponse{StatusCode: http.StatusTooManyRequests},
	}
	got := c.Classify(ctx, "", resp, nil)
	if got != nodepool.NodeStatusBizError {
		t.Errorf("Classify(429 as biz) = %d, want NodeStatusBizError", got)
	}
}

func TestClassifier_CustomClassifyFunc(t *testing.T) {
	c := NewClassifier(
		WithCustomClassify(func(statusCode int, body []byte, err error) nodepool.NodeStatus {
			if statusCode == 200 && string(body) == `{"code":-1}` {
				return nodepool.NodeStatusBizError
			}
			return nodepool.NodeStatusSuccess
		}),
	)
	ctx := context.Background()

	resp := &nodepool.Response{
		Data: &HTTPResponse{StatusCode: 200, Body: []byte(`{"code":-1}`)},
	}
	got := c.Classify(ctx, "", resp, nil)
	if got != nodepool.NodeStatusBizError {
		t.Errorf("Classify(custom) = %d, want NodeStatusBizError", got)
	}
}

// =============================================================================
// HTTPResponse.String 测试
// =============================================================================

func TestHTTPResponse_String(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{"空响应", nil, ""},
		{"有内容", []byte(`{"ok":true}`), `{"ok":true}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &HTTPResponse{Body: tt.body}
			if got := r.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Options 默认值测试
// =============================================================================

func TestDefaultOptions(t *testing.T) {
	transport := New()
	if transport.opts.client == nil {
		t.Error("默认 client 不应为 nil")
	}
	if !transport.opts.keepLog {
		t.Error("默认 keepLog 应为 true")
	}
	if !transport.opts.sendErrorMsg {
		t.Error("默认 sendErrorMsg 应为 true")
	}
}

func TestWithTimeout(t *testing.T) {
	transport := New(WithTimeout(5 * time.Second))
	if transport.opts.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", transport.opts.timeout)
	}
}
