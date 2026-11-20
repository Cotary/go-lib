package log

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/log/handler"
)

// ---------- helpers ----------

func newFile(t *testing.T) (*os.File, string) {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "test.log"))
	if err != nil {
		t.Fatalf("create log file: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f, f.Name()
}

// newLogger 创建写文件的 logger；mk 为 nil 时使用 Raw。
func newLogger(t *testing.T, level slog.Level, mk func(io.Writer, slog.Level) Factory) (*slog.Logger, string) {
	t.Helper()
	f, path := newFile(t)
	factory := Raw(f, level)
	if mk != nil {
		factory = mk(f, level)
	}
	lg, err := New(factory)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return lg, path
}

func useJSON(w io.Writer, l slog.Level) Factory { return JSON(w, l) }
func useText(w io.Writer, l slog.Level) Factory { return Text(w, l) }

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

// ---------- 输出 ----------

func TestRawDefault(t *testing.T) {
	lg, path := newLogger(t, slog.LevelDebug, nil)
	lg.Info("hello", "k", "v")
	lg.Debug("second")

	if got, want := readAll(t, path), "hello\nsecond\n"; got != want {
		t.Fatalf("raw output mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestRawLevelFiltering(t *testing.T) {
	lg, path := newLogger(t, slog.LevelInfo, nil)
	lg.Debug("dropped")
	lg.Info("kept")

	if got, want := readAll(t, path), "kept\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRawKeepsInternalNewlines(t *testing.T) {
	lg, path := newLogger(t, slog.LevelDebug, nil)
	lg.Info("line1\nline2\n")

	if got, want := readAll(t, path), "line1\nline2\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestJSONFactory(t *testing.T) {
	lg, path := newLogger(t, slog.LevelDebug, useJSON)
	lg.Info("json", "k", "v")

	recs := readRecords(t, path)
	if recs[0]["msg"] != "json" || recs[0]["k"] != "v" {
		t.Fatalf("unexpected record: %v", recs[0])
	}
	if recs[0]["level"] != "INFO" {
		t.Fatalf("level should be slog style: %v", recs[0]["level"])
	}
}

func TestTextFactory(t *testing.T) {
	lg, path := newLogger(t, slog.LevelDebug, useText)
	lg.Info("plain")

	raw := readAll(t, path)
	if !strings.Contains(raw, "level=INFO") || !strings.Contains(raw, "plain") {
		t.Fatalf("unexpected text output: %q", raw)
	}
}

func TestMultipleFactories(t *testing.T) {
	var a, b bytes.Buffer
	lg, err := New(JSON(&a, slog.LevelInfo), JSON(&b, slog.LevelInfo))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	lg.Info("fanout")

	if a.Len() == 0 || b.Len() == 0 {
		t.Fatalf("both handlers should receive the record: a=%q b=%q", a.String(), b.String())
	}
}

func TestNilFactoryResult(t *testing.T) {
	_, err := New(func() slog.Handler { return nil })
	if err == nil {
		t.Fatal("nil handler from factory should return error")
	}
}

// ---------- Duration ----------

func TestDurationInSeconds(t *testing.T) {
	lg, path := newLogger(t, slog.LevelDebug, useJSON)
	lg.Info("query", slog.Duration("cost", 1234*time.Millisecond))

	if recs := readRecords(t, path); recs[0]["cost"] != 1.234 {
		t.Fatalf("duration should be seconds, got %v", recs[0]["cost"])
	}
}

func TestDurationInGroup(t *testing.T) {
	lg, path := newLogger(t, slog.LevelDebug, useJSON)
	lg.Info("query", slog.Group("db", slog.Duration("wait", 2*time.Second)))

	recs := readRecords(t, path)
	db, _ := recs[0]["db"].(map[string]any)
	if db["wait"] != float64(2) {
		t.Fatalf("nested duration should be seconds, got %v", recs[0]["db"])
	}
}

// ---------- 组合能力 ----------

func newJSONHandler(w io.Writer) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
}

func TestContextOffByDefault(t *testing.T) {
	lg, path := newLogger(t, slog.LevelDebug, useJSON)
	ctx := context.WithValue(context.Background(), defined.RequestID, "req-1")
	lg.InfoContext(ctx, "no ctx fields")

	if _, ok := readRecords(t, path)[0]["request_id"]; ok {
		t.Fatal("request_id should not be injected without NewContext")
	}
}

func TestContextComposed(t *testing.T) {
	lg, path := newLogger(t, slog.LevelDebug, func(w io.Writer, _ slog.Level) Factory {
		return func() slog.Handler {
			return handler.Chain(newJSONHandler(w),
				handler.NewContext(handler.ContextField{Key: defined.RequestID, Name: "request_id"}),
			)
		}
	})

	ctx := context.WithValue(context.Background(), defined.RequestID, "req-1")
	lg.InfoContext(ctx, "ctx log")
	lg.Info("no ctx")

	recs := readRecords(t, path)
	if recs[0]["request_id"] != "req-1" {
		t.Fatalf("request_id missing: %v", recs[0])
	}
	if _, ok := recs[1]["request_id"]; ok {
		t.Fatalf("request_id should not leak: %v", recs[1])
	}
}

func TestRedactComposed(t *testing.T) {
	lg, path := newLogger(t, slog.LevelDebug, func(w io.Writer, _ slog.Level) Factory {
		return func() slog.Handler {
			return handler.Chain(newJSONHandler(w), handler.NewRedact("password", "token"))
		}
	})

	lg.Info("login", "password", "p@ss", slog.Group("auth", slog.String("token", "t-1")))

	recs := readRecords(t, path)
	if recs[0]["password"] != "***" {
		t.Fatalf("password not redacted: %v", recs[0])
	}
	auth, _ := recs[0]["auth"].(map[string]any)
	if auth["token"] != "***" {
		t.Fatalf("nested token not redacted: %v", recs[0])
	}
}

func TestRedactBeforeAlert(t *testing.T) {
	al := &fakeAlerter{}
	lg, _ := newLogger(t, slog.LevelDebug, func(w io.Writer, _ slog.Level) Factory {
		return func() slog.Handler {
			return handler.Chain(newJSONHandler(w),
				handler.NewRedact("token"),
				handler.NewAlert(al),
			)
		}
	})

	lg.With(AlertKey, true).Error("boom", "token", "secret")

	al.mu.Lock()
	defer al.mu.Unlock()
	if len(al.alerts) != 1 {
		t.Fatalf("want 1 alert, got %d", len(al.alerts))
	}
	if got := al.alerts[0].Attrs[0].Value.String(); got != "***" {
		t.Fatalf("alert attrs should be redacted, got %q", got)
	}
}

type fakeAlerter struct {
	mu     sync.Mutex
	alerts []Alert
}

func (f *fakeAlerter) Send(_ context.Context, a Alert) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alerts = append(f.alerts, a)
	return nil
}

func TestAlertComposed(t *testing.T) {
	al := &fakeAlerter{}
	lg, path := newLogger(t, slog.LevelDebug, func(w io.Writer, _ slog.Level) Factory {
		return func() slog.Handler {
			return handler.Chain(newJSONHandler(w), handler.NewAlert(al))
		}
	})

	lg.With(AlertKey, true).Error("pay_failed", "order_id", 42)
	lg.Info("normal")

	al.mu.Lock()
	n := len(al.alerts)
	al.mu.Unlock()
	if n != 1 {
		t.Fatalf("want 1 alert, got %d", n)
	}
	if strings.Contains(readAll(t, path), AlertKey) {
		t.Fatal("alert marker should not be logged")
	}
}

func TestAlertMarkerStrippedWithoutAlerter(t *testing.T) {
	lg, path := newLogger(t, slog.LevelDebug, useJSON)
	lg.With(AlertKey, true).Warn("no alerter")

	if strings.Contains(readAll(t, path), AlertKey) {
		t.Fatal("alert marker should be stripped even without alerter")
	}
}

func TestSamplingComposed(t *testing.T) {
	lg, path := newLogger(t, slog.LevelDebug, func(w io.Writer, _ slog.Level) Factory {
		return func() slog.Handler {
			return handler.Chain(newJSONHandler(w), handler.NewSample(3, slog.LevelWarn))
		}
	})

	for i := 0; i < 30; i++ {
		lg.Info("sampled", "i", i)
	}
	for i := 0; i < 5; i++ {
		lg.Warn("always", "i", i)
	}

	if recs := readRecords(t, path); len(recs) != 15 {
		t.Fatalf("want 15 records, got %d", len(recs))
	}
}

func TestNewWriter(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(RotateConfig{
		Path:       dir,
		FileName:   "app",
		FileSuffix: ".log",
	})

	lg, err := New(JSON(w, slog.LevelInfo))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	lg.Info("rotate")
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	path := filepath.Join(dir, "app.log")
	if !strings.Contains(readAll(t, path), `"msg":"rotate"`) {
		t.Fatalf("record not written: %s", readAll(t, path))
	}
}

// ---------- 辅助 ----------

func TestWithFields(t *testing.T) {
	lg, path := newLogger(t, slog.LevelDebug, useJSON)
	lg.With(WithFields(map[string]any{"b": 2, "a": 1})...).Info("map fields")

	recs := readRecords(t, path)
	if recs[0]["a"] != float64(1) || recs[0]["b"] != float64(2) {
		t.Fatalf("WithFields lost fields: %v", recs[0])
	}
}

// ---------- 级别解析 ----------

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"TRACE":   slog.LevelDebug,
		"verbose": slog.LevelDebug,
		"info":    slog.LevelInfo,
		" notice": slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"fatal":   slog.LevelError,
		"":        slog.LevelInfo,
		"nope":    slog.LevelInfo,
	}
	for in, want := range cases {
		if got := ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestUnknownLevelFallsBackToInfo(t *testing.T) {
	lg, path := newLogger(t, ParseLevel("nope"), nil)
	lg.Debug("dropped")
	lg.Info("kept")

	if got, want := readAll(t, path), "kept\n"; got != want {
		t.Fatalf("unknown level should fall back to info: got %q, want %q", got, want)
	}
}

// ---------- 并发 ----------

func TestConcurrentLogging(t *testing.T) {
	lg, path := newLogger(t, slog.LevelDebug, useJSON)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			lg.With("goroutine", n).Info("concurrent")
		}(i)
	}
	wg.Wait()

	if recs := readRecords(t, path); len(recs) != 50 {
		t.Fatalf("want 50 records, got %d", len(recs))
	}
}
