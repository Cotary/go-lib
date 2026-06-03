package coroutines

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// resetCrashStateForTest 把包级状态清零，让多个测试可以独立 InitCrashReporter。
// 仅在测试中调用；生产代码绝不应触碰这些字段。
//
// 必须先 SetCrashOutput(nil) 解除 runtime 对 fd 的持有，再 Close —— 尤其在
// Windows 上，否则 t.TempDir 清理时会因为文件仍被占用而 RemoveAll 失败。
func resetCrashStateForTest(t *testing.T) {
	t.Helper()
	crashMu.Lock()
	if crashFile != nil {
		_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
		_ = crashFile.Close()
	}
	crashCfg = nil
	crashFile = nil
	crashFilePid = 0
	crashMu.Unlock()
}

// TestInitCrashReporter_Idempotent 验证重复调用不会出错也不会覆盖现有 fd。
func TestInitCrashReporter_Idempotent(t *testing.T) {
	defer resetCrashStateForTest(t)
	resetCrashStateForTest(t)

	dir := t.TempDir()
	if err := InitCrashReporter(WithCrashDir(dir)); err != nil {
		t.Fatalf("first init: %v", err)
	}
	first := crashFile

	if err := InitCrashReporter(WithCrashDir(t.TempDir())); err != nil {
		t.Fatalf("second init: %v", err)
	}
	if crashFile != first {
		t.Fatalf("crash file fd should not change on second init")
	}
}

// TestParseCrashFile_TruncateAndReason 构造一份伪造 dump，验证截断与 Reason 解析。
func TestParseCrashFile_TruncateAndReason(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "1234-20240101_010203.crash")
	body := "fatal error: concurrent map writes\n\ngoroutine 1 [running]:\n" + strings.Repeat("x", 1024)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	rec, err := parseCrashFile(path, 64)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !rec.Truncated {
		t.Fatalf("expected truncated")
	}
	if rec.Reason != "fatal error: concurrent map writes" {
		t.Fatalf("unexpected reason: %q", rec.Reason)
	}
	if rec.Pid != 1234 {
		t.Fatalf("unexpected pid: %d", rec.Pid)
	}
	if rec.StartedAt.IsZero() {
		t.Fatalf("startedAt should not be zero")
	}
	if int64(len(rec.Raw)) != 64 {
		t.Fatalf("raw len should be 64, got %d", len(rec.Raw))
	}
}

// TestReportPendingCrashes_UploadsAndRenames 验证默认流程：
//   - 历史 dump 触发 uploader
//   - 当前进程文件被排除
//   - 空 dump（正常退出时残留）直接归档不上报
//   - 上报成功后重命名为 .uploaded
func TestReportPendingCrashes_UploadsAndRenames(t *testing.T) {
	defer resetCrashStateForTest(t)
	resetCrashStateForTest(t)

	dir := t.TempDir()

	// 历史 dump：另一个 pid 的真实 fatal
	historic := filepath.Join(dir, "9999-20240101_010101.crash")
	if err := os.WriteFile(historic, []byte("fatal error: concurrent map writes\nstack...\n"), 0o600); err != nil {
		t.Fatalf("write historic: %v", err)
	}

	// 模拟"当前进程正在写"的文件，扫描时应跳过
	curName := fmt.Sprintf("%d-20240101_020202.crash", os.Getpid())
	curPath := filepath.Join(dir, curName)
	if err := os.WriteFile(curPath, []byte("not-a-real-dump"), 0o600); err != nil {
		t.Fatalf("write current: %v", err)
	}

	// 空 dump（上次进程正常退出，runtime 没写）
	empty := filepath.Join(dir, "8888-20240101_030303.crash")
	if err := os.WriteFile(empty, []byte{}, 0o600); err != nil {
		t.Fatalf("write empty: %v", err)
	}

	var got []*CrashRecord
	uploader := func(ctx context.Context, rec *CrashRecord) error {
		got = append(got, rec)
		return nil
	}

	// 直接构造 cfg 跳过 SetCrashOutput，避免污染全局 runtime 状态
	crashMu.Lock()
	crashCfg = &crashConfig{
		dir:          dir,
		fileMode:     0o640,
		traceback:    "all",
		maxFileBytes: 256 * 1024,
		keepUploaded: time.Hour,
		title:        "Process Crashed",
		uploader:     uploader,
	}
	crashFilePid = os.Getpid()
	crashMu.Unlock()

	if err := ReportPendingCrashes(context.Background()); err != nil {
		t.Fatalf("report: %v", err)
	}

	if len(got) != 1 || got[0].Pid != 9999 {
		t.Fatalf("expected exactly 1 historic record (pid=9999), got %+v", got)
	}

	if _, err := os.Stat(historic + uploadedSuffix); err != nil {
		t.Fatalf("historic should be renamed to .uploaded: %v", err)
	}
	if _, err := os.Stat(empty + uploadedSuffix); err != nil {
		t.Fatalf("empty should be renamed to .uploaded (silent archive): %v", err)
	}
	if _, err := os.Stat(curPath); err != nil {
		t.Fatalf("current pid file should remain untouched: %v", err)
	}
}

// TestReportPendingCrashes_UploaderErrorPreservesFile 验证 uploader 失败时不归档，便于下次重试。
func TestReportPendingCrashes_UploaderErrorPreservesFile(t *testing.T) {
	defer resetCrashStateForTest(t)
	resetCrashStateForTest(t)

	dir := t.TempDir()
	dump := filepath.Join(dir, "7777-20240101_010101.crash")
	if err := os.WriteFile(dump, []byte("fatal error: deadlock\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	wantErr := errors.New("network down")
	crashMu.Lock()
	crashCfg = &crashConfig{
		dir:          dir,
		maxFileBytes: 1024,
		keepUploaded: time.Hour,
		uploader:     func(ctx context.Context, rec *CrashRecord) error { return wantErr },
	}
	crashFilePid = os.Getpid()
	crashMu.Unlock()

	err := ReportPendingCrashes(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrap of wantErr, got %v", err)
	}
	if _, err := os.Stat(dump); err != nil {
		t.Fatalf("dump should be preserved on uploader error: %v", err)
	}
	if _, err := os.Stat(dump + uploadedSuffix); err == nil {
		t.Fatalf(".uploaded should NOT exist when uploader failed")
	}
}

// TestCleanupUploaded_RemovesExpired 验证过期 .uploaded 会被清理，新文件会保留。
func TestCleanupUploaded_RemovesExpired(t *testing.T) {
	dir := t.TempDir()

	old := filepath.Join(dir, "1-20240101_010101.crash.uploaded")
	fresh := filepath.Join(dir, "2-20240101_020202.crash.uploaded")
	if err := os.WriteFile(old, []byte("x"), 0o600); err != nil {
		t.Fatalf("write old: %v", err)
	}
	if err := os.WriteFile(fresh, []byte("x"), 0o600); err != nil {
		t.Fatalf("write fresh: %v", err)
	}
	// 把 old 的 mtime 调到很久以前
	past := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	cleanupUploaded(dir, 7*24*time.Hour)

	if _, err := os.Stat(old); err == nil {
		t.Fatalf("old .uploaded should be removed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh .uploaded should be kept: %v", err)
	}
}

// TestCrashReporter_E2E_ConcurrentMapWrite 端到端验证：
// 父进程 fork 出一个子进程触发 concurrent map writes，子进程 InitCrashReporter
// 后必死无疑，父进程读 dump 文件断言其包含 "fatal error: concurrent map"。
//
// 子进程入口靠 "GO_LIB_CRASH_E2E=1" 环境变量切换；测试 -test.run 会被 Go test
// 二进制识别，从而执行 helper 分支。
func TestCrashReporter_E2E_ConcurrentMapWrite(t *testing.T) {
	if os.Getenv("GO_LIB_CRASH_E2E") == "1" {
		runCrashHelper()
		return
	}
	if testing.Short() {
		t.Skip("skip E2E in -short mode")
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run", "TestCrashReporter_E2E_ConcurrentMapWrite", "-test.timeout=20s")
	cmd.Env = append(os.Environ(), "GO_LIB_CRASH_E2E=1", "GO_LIB_CRASH_DIR="+dir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("child should have crashed; stdout=%s", out)
	}

	// 子进程崩溃后 dump 文件应在 dir 中
	deadline := time.Now().Add(5 * time.Second)
	var found string
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), crashFileSuffix) {
				found = filepath.Join(dir, e.Name())
				break
			}
		}
		if found != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if found == "" {
		t.Fatalf("no .crash file generated in %s; child stdout=%s", dir, out)
	}
	body, err := os.ReadFile(found)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	// runtime 在 fatal 时通过 print() 直接把 "fatal error: xxx" 写到 fd 2，
	// SetCrashOutput 收到的只是后续的 goroutine 栈 dump。所以这里检查能反推
	// 出根因的栈帧名（concurrent map writes 由 internal/runtime/maps.fatal 抛出）
	// 以及通用的 goroutine traceback 标记，而不是 "fatal error:" 字面量。
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "internal/runtime/maps.fatal") && !strings.Contains(bodyStr, "concurrent map") {
		t.Fatalf("dump does not contain expected fatal marker; body=%s", body)
	}
	if !strings.Contains(bodyStr, "goroutine ") {
		t.Fatalf("dump should contain goroutine traceback; body=%s", body)
	}
}

// runCrashHelper 是子进程入口：注册 crash reporter 然后并发写 map 触发 fatal。
// 注意：concurrent map writes 是 runtime 探测到才会 throw，循环必须足够紧。
func runCrashHelper() {
	dir := os.Getenv("GO_LIB_CRASH_DIR")
	if err := InitCrashReporter(WithCrashDir(dir)); err != nil {
		fmt.Fprintln(os.Stderr, "init crash reporter:", err)
		os.Exit(99)
	}
	m := make(map[int]int)
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					for k := 0; k < 1000; k++ {
						m[k] = k
					}
				}
			}
		}()
	}
	// 兜底：万一 runtime 没探测到，超时退出避免 hang 死父进程
	time.Sleep(10 * time.Second)
	close(done)
	os.Exit(0)
}
