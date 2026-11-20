package log

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Cotary/go-lib/common/defined"
)

// ---------- helpers ----------

func newTestLogger(t *testing.T, cfg Config, opts ...Option) (*slog.Logger, string) {
	t.Helper()
	if cfg.Path == "" {
		cfg.Path = t.TempDir()
	}
	if cfg.FileName == "" {
		cfg.FileName = "test"
	}
	if cfg.Level == "" {
		cfg.Level = "debug"
	}
	lg, sink, err := New(cfg, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	return lg, filepath.Join(cfg.Path, cfg.FileName+".logger")
}

func readAll(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	return string(b)
}

func readRecords(t *testing.T, path string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(readAll(t, path)), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("parse line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

// ---------- 输出与过滤 ----------

func TestLevelFiltering(t *testing.T) {
	lg, path := newTestLogger(t, Config{Level: "info"})
	lg.Debug("dropped")
	lg.Info("kept")

	recs := readRecords(t, path)
	if len(recs) != 1 || recs[0]["msg"] != "kept" {
		t.Fatalf("want only info record, got %v", recs)
	}
}

func TestTextFormat(t *testing.T) {
	lg, path := newTestLogger(t, Config{Format: FormatText})
	lg.Info("plain text message")

	raw := strings.TrimSpace(readAll(t, path))
	if strings.HasPrefix(raw, "{") {
		t.Fatalf("expected text output, got JSON: %s", raw)
	}
	if !strings.Contains(raw, "level=INFO") {
		t.Fatalf("text format should use slog TextHandler: %q", raw)
	}
	if !strings.Contains(raw, "plain text message") {
		t.Fatalf("message missing: %s", raw)
	}
}

func TestDurationInSeconds(t *testing.T) {
	lg, path := newTestLogger(t, Config{})
	lg.Info("query", slog.Duration("cost", 1234*time.Millisecond))

	recs := readRecords(t, path)
	if got := recs[0]["cost"]; got != 1.234 {
		t.Fatalf("duration should be seconds, got %v", got)
	}
}

// 没有包装层，source 必须落在业务代码（本测试文件）上，而不是 log 包内部。
func TestSourcePointsToCaller(t *testing.T) {
	lg, path := newTestLogger(t, Config{ShowFile: true})

	_, _, wantLine, _ := runtime.Caller(0)
	lg.Info("source check")

	recs := readRecords(t, path)
	src, ok := recs[0]["source"].(map[string]any)
	if !ok {
		t.Fatalf("missing source attr: %v", recs[0])
	}
	if file, _ := src["file"].(string); !strings.HasSuffix(file, "logger_test.go") {
		t.Fatalf("source file should be the caller, got %v", src["file"])
	}
	if line, _ := src["line"].(float64); int(line) != wantLine+1 {
		t.Fatalf("source line should be %d, got %v", wantLine+1, src["line"])
	}
	if fn, _ := src["function"].(string); !strings.Contains(fn, "TestSourcePointsToCaller") {
		t.Fatalf("source function should be the test, got %v", src["function"])
	}
}

// ---------- 派生字段 ----------

func TestWithDerivation(t *testing.T) {
	lg, path := newTestLogger(t, Config{})
	lg.With("module", "pay").WithGroup("db").Info("query", "sql", "select 1")

	recs := readRecords(t, path)
	db, ok := recs[0]["db"].(map[string]any)
	if !ok || db["sql"] != "select 1" {
		t.Fatalf("grouped attrs missing: %v", recs[0])
	}
	if recs[0]["module"] != "pay" {
		t.Fatalf("derived attr missing: %v", recs[0])
	}
}

func TestFixedFields(t *testing.T) {
	lg, path := newTestLogger(t, Config{}, WithFixedFields(map[string]any{"service": "api", "env": "test"}))
	lg.Info("x")

	recs := readRecords(t, path)
	if recs[0]["service"] != "api" || recs[0]["env"] != "test" {
		t.Fatalf("fixed fields missing: %v", recs[0])
	}
}

// ---------- request_id ----------

func TestRequestIDFromContext(t *testing.T) {
	lg, path := newTestLogger(t, Config{})
	ctx := context.WithValue(context.Background(), defined.RequestID, "req-1")

	lg.InfoContext(ctx, "ctx log")
	lg.Info("no ctx")

	recs := readRecords(t, path)
	if recs[0]["request_id"] != "req-1" {
		t.Fatalf("request_id not injected: %v", recs[0])
	}
	if _, ok := recs[1]["request_id"]; ok {
		t.Fatalf("request_id should not leak to records without ctx: %v", recs[1])
	}
}

func TestRequestIDNotDuplicated(t *testing.T) {
	lg, path := newTestLogger(t, Config{})
	ctx := context.WithValue(context.Background(), defined.RequestID, "req-1")

	lg.With(defined.RequestID, "explicit").InfoContext(ctx, "explicit")

	raw := readAll(t, path)
	if n := strings.Count(raw, `"request_id"`); n != 1 {
		t.Fatalf("request_id should appear once, got %d: %s", n, raw)
	}
}

func TestRequestIDMultipleKeys(t *testing.T) {
	lg, path := newTestLogger(t, Config{},
		WithRequestIDKeys(defined.RequestID, defined.TransactionID, defined.RequestURI))

	ctx := context.Background()
	ctx = context.WithValue(ctx, defined.RequestID, "req-1")
	ctx = context.WithValue(ctx, defined.TransactionID, "tx-1")

	lg.InfoContext(ctx, "multi keys")

	recs := readRecords(t, path)
	if recs[0]["request_id"] != "req-1" {
		t.Fatalf("request_id missing: %v", recs[0])
	}
	if recs[0]["transaction_id"] != "tx-1" {
		t.Fatalf("transaction_id missing: %v", recs[0])
	}
	if _, ok := recs[0]["request_uri"]; ok {
		t.Fatalf("absent key should be skipped: %v", recs[0])
	}
}

func TestRequestIDKeysWithAttrsWins(t *testing.T) {
	lg, path := newTestLogger(t, Config{},
		WithRequestIDKeys(defined.RequestID, defined.TransactionID))

	ctx := context.Background()
	ctx = context.WithValue(ctx, defined.RequestID, "from-ctx")
	ctx = context.WithValue(ctx, defined.TransactionID, "from-ctx")

	lg.With(defined.TransactionID, "explicit").InfoContext(ctx, "dedup")

	raw := readAll(t, path)
	if n := strings.Count(raw, `"transaction_id"`); n != 1 {
		t.Fatalf("transaction_id should appear once, got %d: %s", n, raw)
	}
	if !strings.Contains(raw, `"transaction_id":"explicit"`) {
		t.Fatalf("explicit value should win: %s", raw)
	}
}

type traceCtxKey struct{}

func TestContextFieldsWithTypedKey(t *testing.T) {
	lg, path := newTestLogger(t, Config{}, WithContextFields(
		ContextField{Key: defined.RequestID, Name: "request_id"},
		ContextField{Key: traceCtxKey{}, Name: "trace_id"},
	))

	ctx := context.Background()
	ctx = context.WithValue(ctx, defined.RequestID, "req-1")
	ctx = context.WithValue(ctx, traceCtxKey{}, "trace-1")

	lg.InfoContext(ctx, "typed key")

	recs := readRecords(t, path)
	if recs[0]["request_id"] != "req-1" || recs[0]["trace_id"] != "trace-1" {
		t.Fatalf("typed ctx key not injected: %v", recs[0])
	}
}

func TestRequestIDKeysEmptyDisables(t *testing.T) {
	lg, path := newTestLogger(t, Config{}, WithRequestIDKeys())

	ctx := context.WithValue(context.Background(), defined.RequestID, "req-1")
	lg.InfoContext(ctx, "disabled")

	if recs := readRecords(t, path); recs[0]["request_id"] != nil {
		t.Fatalf("request_id should not be injected: %v", recs[0])
	}
}

func TestNilContextIsSafe(t *testing.T) {
	lg, path := newTestLogger(t, Config{})
	lg.InfoContext(nil, "nil ctx") //nolint:staticcheck // 验证标准库已有兜底

	if recs := readRecords(t, path); len(recs) != 1 {
		t.Fatalf("want 1 record, got %v", recs)
	}
}

// ---------- 脱敏 ----------

func TestRedact(t *testing.T) {
	lg, path := newTestLogger(t, Config{})
	lg.Info("login",
		"user", "bob",
		"password", "p@ss",
		slog.Group("auth", slog.String("token", "t-1")),
	)

	recs := readRecords(t, path)
	if recs[0]["password"] != "***" {
		t.Fatalf("password not redacted: %v", recs[0])
	}
	auth, ok := recs[0]["auth"].(map[string]any)
	if !ok || auth["token"] != "***" {
		t.Fatalf("nested token not redacted: %v", recs[0])
	}
	if recs[0]["user"] != "bob" {
		t.Fatalf("non-sensitive attr should be kept: %v", recs[0])
	}
}

func TestRedactWithAttrs(t *testing.T) {
	lg, path := newTestLogger(t, Config{}, WithRedactKeys("api_key"))
	lg.With("api_key", "k-1").Info("x")

	recs := readRecords(t, path)
	if recs[0]["api_key"] != "***" {
		t.Fatalf("With attr not redacted: %v", recs[0])
	}
}

// ---------- 告警 ----------

type fakeAlerter struct {
	mu     sync.Mutex
	alerts []Alert
	err    error
}

func (f *fakeAlerter) Send(_ context.Context, a Alert) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alerts = append(f.alerts, a)
	return f.err
}

func (f *fakeAlerter) snapshot() []Alert {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Alert(nil), f.alerts...)
}

func TestAlert(t *testing.T) {
	al := &fakeAlerter{}
	lg, path := newTestLogger(t, Config{}, WithAlerter(al))

	lg.With(AlertKey, true).Error("pay_failed", "order_id", 42)
	lg.Info("just a log") // 不应触发告警

	alerts := al.snapshot()
	if len(alerts) != 1 {
		t.Fatalf("want 1 alert, got %d", len(alerts))
	}
	if alerts[0].Title != "pay_failed" || alerts[0].Level != slog.LevelError {
		t.Fatalf("unexpected alert: %+v", alerts[0])
	}
	if alerts[0].Fingerprint() == "" {
		t.Fatal("fingerprint should not be empty")
	}

	raw := readAll(t, path)
	if strings.Contains(raw, AlertKey) {
		t.Fatalf("alert marker should not be logged: %s", raw)
	}
	if !strings.Contains(raw, `"order_id":42`) {
		t.Fatalf("alert record should still be logged: %s", raw)
	}
}

func TestAlertPerCallFlag(t *testing.T) {
	al := &fakeAlerter{}
	lg, _ := newTestLogger(t, Config{}, WithAlerter(al))

	lg.Warn("disk_full", slog.Bool(AlertKey, true))
	if got := len(al.snapshot()); got != 1 {
		t.Fatalf("want 1 alert, got %d", got)
	}
}

func TestAlertSendErrorIsLogged(t *testing.T) {
	al := &fakeAlerter{err: fmt.Errorf("boom")}
	lg, path := newTestLogger(t, Config{}, WithAlerter(al))

	lg.With(AlertKey, true).Error("failed", "k", "v")

	raw := readAll(t, path)
	if !strings.Contains(raw, "alert send failed") {
		t.Fatalf("send error should be logged: %s", raw)
	}
}

// ---------- 采样 ----------

func TestSampling(t *testing.T) {
	lg, path := newTestLogger(t, Config{}, WithSampling(3, slog.LevelWarn))

	for i := 0; i < 30; i++ {
		lg.Info("sampled", "i", i)
	}
	for i := 0; i < 5; i++ {
		lg.Warn("always", "i", i)
	}

	recs := readRecords(t, path)
	if len(recs) != 10+5 {
		t.Fatalf("want 15 records (10 sampled info + 5 warn), got %d", len(recs))
	}
}

// ---------- Sink ----------

func TestSinkStdoutOnly(t *testing.T) {
	_, sink, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if sink.closer != nil {
		t.Fatal("Path empty should not create a file writer")
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSinkClose(t *testing.T) {
	lg, path := newTestLogger(t, Config{})
	lg.Info("before close")
	if !strings.Contains(readAll(t, path), "before close") {
		t.Fatal("record should be written")
	}
}

// ---------- 全局 ----------

func TestWithFields(t *testing.T) {
	lg, path := newTestLogger(t, Config{})
	lg.With(WithFields(map[string]any{"b": 2, "a": 1})...).Info("map fields")

	recs := readRecords(t, path)
	if recs[0]["a"] != float64(1) || recs[0]["b"] != float64(2) {
		t.Fatalf("WithFields lost fields: %v", recs[0])
	}

	raw := readAll(t, path)
	if strings.Index(raw, `"a":1`) > strings.Index(raw, `"b":2`) {
		t.Fatalf("fields should be sorted: %s", raw)
	}
}

// ---------- 配置校验 ----------

func TestUnknownLevel(t *testing.T) {
	if _, _, err := New(Config{Level: "nope"}); err == nil {
		t.Fatal("unknown level should return error")
	}
}

func TestLevelAliases(t *testing.T) {
	for _, name := range []string{"trace", "verbose", "DEBUG", " info ", "warning", "fatal"} {
		if _, _, err := New(Config{Level: name}); err != nil {
			t.Fatalf("level %q should be accepted: %v", name, err)
		}
	}
}

// ---------- 并发 ----------

func TestConcurrentLogging(t *testing.T) {
	lg, path := newTestLogger(t, Config{})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ctx := context.WithValue(context.Background(), defined.RequestID, fmt.Sprintf("req-%d", n))
			lg.With("goroutine", n).InfoContext(ctx, "concurrent")
		}(i)
	}
	wg.Wait()

	if recs := readRecords(t, path); len(recs) != 50 {
		t.Fatalf("want 50 records, got %d", len(recs))
	}
}
