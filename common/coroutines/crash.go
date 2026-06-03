package coroutines

// 进程级 fatal error 兜底告警实现。
//
// 设计动机
//
// Go 错误按严重度可分为四级，本文件聚焦 L3：
//   - L1 error：业务返回值，调用方 if err != nil 即可。
//   - L2 panic：协程级，运行时异常或显式 panic()，可在当前 goroutine
//     的 defer 中 recover；本包 SafeGo / SafeFunc 已覆盖。
//   - L3 fatal error：runtime.throw / runtime.fatal 路径，例如
//     "concurrent map writes"、"all goroutines are asleep - deadlock!"、
//     "stack overflow"、"runtime: out of memory"、"sync: unlock of
//     unlocked mutex" 等。runtime 跳过所有 defer，按 GOTRACEBACK 打印
//     全 goroutine 栈后立即 exit(2)。recover() 拿不到，进程内无任何
//     拦截手段。
//   - L4 系统级处决：SIGKILL / OOMKilled / 断电，进程瞬死，零遗言；
//     只能依靠进程外监控（systemd / k8s / Prometheus）。
//
// 本文件解决 L3：利用 Go 1.23+ 的 runtime/debug.SetCrashOutput，让
// runtime 在向 stderr 打印 fatal 信息的同时，把同样的 dump 复制写入
// 我们提供的文件。进程死后，下次启动通过 ReportPendingCrashes 扫描
// dump 目录、把上次崩溃信息走 notify 通道补报。

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Cotary/go-lib/notify"
)

// crashFileSuffix 是 dump 文件扩展名，扫描时严格匹配以避免误读其它日志。
const crashFileSuffix = ".crash"

// uploadedSuffix 是已成功上报后的标记，附加在原文件名后；
// 通过重命名实现"已读"语义，避免重复告警。
const uploadedSuffix = ".uploaded"

// ===== 类型 =====

// CrashRecord 表示一次历史崩溃的解析结果。
//
// File 为绝对路径；Pid / StartedAt 由文件名拆解得到，文件名格式为
// "<pid>-<yyyymmdd_HHMMSS>.crash"，解析失败时这两个字段保持零值。
// Reason 是从 dump 中提取的首行 "fatal error: xxx"，便于告警标题展示；
// Raw 为原始 dump 的字节内容，可能因 maxFileBytes 而被截断，此时
// Truncated 为 true。完整内容仍在 File 中，可供人工排查。
type CrashRecord struct {
	File      string
	Pid       int
	StartedAt time.Time
	Reason    string
	Raw       []byte
	Truncated bool
}

// Render 把 CrashRecord 渲染成给人看的多行文本，
// 用作默认 uploader 注入到 notify 的 error 消息体。
func (r *CrashRecord) Render() string {
	var b strings.Builder
	b.WriteString("Process Crashed\n")
	if r.Pid > 0 {
		fmt.Fprintf(&b, "Pid: %d\n", r.Pid)
	}
	if !r.StartedAt.IsZero() {
		fmt.Fprintf(&b, "StartedAt: %s\n", r.StartedAt.Format(time.RFC3339))
	}
	if r.Reason != "" {
		fmt.Fprintf(&b, "Reason: %s\n", r.Reason)
	}
	fmt.Fprintf(&b, "DumpFile: %s\n", r.File)
	if r.Truncated {
		fmt.Fprintf(&b, "Truncated: true (raw size > maxFileBytes)\n")
	}
	b.WriteString("---- raw dump ----\n")
	b.Write(r.Raw)
	if len(r.Raw) > 0 && r.Raw[len(r.Raw)-1] != '\n' {
		b.WriteByte('\n')
	}
	return b.String()
}

// ===== 配置 =====

// crashConfig 是不可导出的内部配置，所有字段均通过 With* 选项注入，
// 保持 Functional Options 风格，业务零配置即可使用。
type crashConfig struct {
	dir          string
	fileMode     os.FileMode
	traceback    string
	maxFileBytes int64
	keepUploaded time.Duration
	title        string
	uploader     func(context.Context, *CrashRecord) error
}

// CrashOption 配置 InitCrashReporter 行为；
// 非法参数（空目录、负值等）会被忽略以保持库的健壮性。
type CrashOption func(*crashConfig)

// WithCrashDir 指定 dump 文件目录，默认 "./logs/crash"。
// 容器场景务必映射到持久卷，否则 cgroup 销毁时文件会丢。
func WithCrashDir(dir string) CrashOption {
	return func(c *crashConfig) {
		if strings.TrimSpace(dir) != "" {
			c.dir = dir
		}
	}
}

// WithTraceback 设置 GOTRACEBACK，影响 fatal 时打印的栈范围；
// 合法值 "single"/"all"/"system"/"crash"。注意 runtime 只允许
// 升高详细度，传 "none" 会被忽略。默认 "all"。
func WithTraceback(level string) CrashOption {
	return func(c *crashConfig) {
		switch level {
		case "single", "all", "system", "crash":
			c.traceback = level
		}
	}
}

// WithMaxFileBytes 限制单个 dump 文件读入告警的最大字节数；
// 超过部分会截断并在 CrashRecord.Truncated 标记，
// 防止超大 dump 把 notify 通道（IM / 邮件）撑爆。默认 256 KB。
func WithMaxFileBytes(n int64) CrashOption {
	return func(c *crashConfig) {
		if n > 0 {
			c.maxFileBytes = n
		}
	}
}

// WithKeepUploaded 指定 .uploaded 文件保留时长，超过后清理。
// 默认 7 天，便于事故复盘时从原始文件查全栈。
func WithKeepUploaded(d time.Duration) CrashOption {
	return func(c *crashConfig) {
		if d > 0 {
			c.keepUploaded = d
		}
	}
}

// WithCrashTitle 覆盖告警标题。默认 "Process Crashed"。
func WithCrashTitle(title string) CrashOption {
	return func(c *crashConfig) {
		if strings.TrimSpace(title) != "" {
			c.title = title
		}
	}
}

// WithCrashUploader 替换默认 uploader（默认走 notify.SendErrMessage）。
// 业务可借此把 dump 直推 Sentry / 钉钉 / 飞书等任意通道。
// 返回 error 表示本条上报失败，框架会保留原文件等待下次启动重试。
func WithCrashUploader(fn func(context.Context, *CrashRecord) error) CrashOption {
	return func(c *crashConfig) {
		if fn != nil {
			c.uploader = fn
		}
	}
}

// ===== 内部状态 =====
//
// crashMu 保护以下三个变量：crashCfg 表示是否已初始化（nil = 未初始化），
// crashFile 是 SetCrashOutput 持有的文件句柄，进程崩溃时由 OS 回收，
// 所以本进程生命周期内绝不主动 Close；crashFilePid 用于
// ReportPendingCrashes 扫描时排除当前进程正在写入的那个文件。
var (
	crashMu      sync.Mutex
	crashCfg     *crashConfig
	crashFile    *os.File
	crashFilePid int
)

// ===== 对外 API =====

// InitCrashReporter 在程序最早期调用一次，幂等。
//
// 调用时机：必须在任何会启动 goroutine 的逻辑之前，否则在注册之前
// 触发的 fatal 仍然只会打到 stderr。建议放在 main 的最前几行。
//
// 副作用：在 dir 下创建一个 "<pid>-<yyyymmdd_HHMMSS>.crash" 文件并把
// fd 注册给 runtime；同时设置 GOTRACEBACK。该文件在进程正常退出时
// 会变成空文件，运维侧可定期清理。
func InitCrashReporter(opts ...CrashOption) error {
	crashMu.Lock()
	defer crashMu.Unlock()

	if crashCfg != nil {
		return nil
	}

	cfg := defaultCrashConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	if err := os.MkdirAll(cfg.dir, 0o750); err != nil {
		return fmt.Errorf("crash: mkdir %q: %w", cfg.dir, err)
	}

	f, err := openCrashFile(cfg)
	if err != nil {
		return err
	}

	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		_ = f.Close()
		return fmt.Errorf("crash: SetCrashOutput: %w", err)
	}

	debug.SetTraceback(cfg.traceback)

	crashCfg = cfg
	crashFile = f
	crashFilePid = os.Getpid()
	return nil
}

// ReportPendingCrashes 扫描 dump 目录，把上一次崩溃留下的 *.crash
// 文件读出 → 调 uploader 上报 → 重命名为 *.crash.uploaded；
// 同时清理过期 .uploaded 文件。
//
// 调用时机：在 message sender 初始化完成（lib.InitGlobalSender）
// 之后调用一次，确保默认 uploader 能把告警发出去。
//
// 行为：当前进程正在写入的那个文件（pid 匹配 crashFilePid）会被跳过；
// 单文件失败不影响其它文件的上报；所有错误聚合后返回第一个，
// 其余写日志。InitCrashReporter 未调用时直接返回 nil 不报错，
// 便于在测试 / 临时关闭崩溃捕获时调用方无需做条件判断。
func ReportPendingCrashes(ctx context.Context) error {
	crashMu.Lock()
	cfg := crashCfg
	currentPid := crashFilePid
	crashMu.Unlock()

	if cfg == nil {
		return nil
	}

	entries, err := os.ReadDir(cfg.dir)
	if err != nil {
		return fmt.Errorf("crash: readdir %q: %w", cfg.dir, err)
	}

	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, crashFileSuffix) {
			continue
		}
		if pid := pidFromName(name); pid == currentPid && currentPid != 0 {
			continue
		}
		files = append(files, filepath.Join(cfg.dir, name))
	}
	sort.Strings(files)

	uploader := cfg.uploader
	if uploader == nil {
		uploader = defaultUploader
	}

	var firstErr error
	for _, path := range files {
		rec, perr := parseCrashFile(path, cfg.maxFileBytes)
		if perr != nil {
			if firstErr == nil {
				firstErr = perr
			}
			continue
		}
		if len(rec.Raw) == 0 {
			// 空文件意味着上次进程是正常退出（没触发 fatal），
			// 这种文件没有告警价值，直接重命名归档，避免下次重复扫描。
			_ = os.Rename(path, path+uploadedSuffix)
			continue
		}
		if uerr := uploader(ctx, rec); uerr != nil {
			if firstErr == nil {
				firstErr = uerr
			}
			continue
		}
		if rerr := os.Rename(path, path+uploadedSuffix); rerr != nil && firstErr == nil {
			firstErr = rerr
		}
	}

	cleanupUploaded(cfg.dir, cfg.keepUploaded)
	return firstErr
}

// ===== 内部实现 =====

// defaultCrashConfig 返回内置默认值，代表"零配置即可用"的最小集。
func defaultCrashConfig() *crashConfig {
	return &crashConfig{
		dir:          "./logs/crash",
		fileMode:     0o640,
		traceback:    "all",
		maxFileBytes: 256 * 1024,
		keepUploaded: 7 * 24 * time.Hour,
		title:        "Process Crashed",
		uploader:     defaultUploader,
	}
}

// openCrashFile 创建当前进程专属的 dump 文件并返回 *os.File。
// 文件名嵌入 pid 与启动时间，支持同主机多实例并发不冲突。
// O_APPEND 是为了让 runtime 安全追加（虽然 runtime 通常一次性写完）。
func openCrashFile(cfg *crashConfig) (*os.File, error) {
	name := fmt.Sprintf("%d-%s%s", os.Getpid(), time.Now().Format("20060102_150405"), crashFileSuffix)
	path := filepath.Join(cfg.dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, cfg.fileMode)
	if err != nil {
		return nil, fmt.Errorf("crash: open %q: %w", path, err)
	}
	return f, nil
}

// pidFromName 从 "<pid>-<ts>.crash" 形式的文件名提取 pid，失败返回 0。
func pidFromName(name string) int {
	base := strings.TrimSuffix(name, crashFileSuffix)
	idx := strings.IndexByte(base, '-')
	if idx <= 0 {
		return 0
	}
	pid, err := strconv.Atoi(base[:idx])
	if err != nil {
		return 0
	}
	return pid
}

// startedAtFromName 从文件名提取启动时间，失败返回零值。
func startedAtFromName(name string) time.Time {
	base := strings.TrimSuffix(name, crashFileSuffix)
	idx := strings.IndexByte(base, '-')
	if idx < 0 || idx+1 >= len(base) {
		return time.Time{}
	}
	t, err := time.ParseInLocation("20060102_150405", base[idx+1:], time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// parseCrashFile 读取 dump 文件并提取关键字段。
// 超出 maxFileBytes 时只读前 max 字节并标记 Truncated；
// Reason 取 dump 中第一行匹配 "fatal error:" 或 "panic:" 的内容。
func parseCrashFile(path string, maxBytes int64) (*CrashRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("crash: open dump %q: %w", path, err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("crash: stat %q: %w", path, err)
	}

	size := st.Size()
	readSize := size
	truncated := false
	if maxBytes > 0 && size > maxBytes {
		readSize = maxBytes
		truncated = true
	}

	buf := make([]byte, readSize)
	if _, err := f.Read(buf); err != nil && readSize > 0 {
		return nil, fmt.Errorf("crash: read %q: %w", path, err)
	}

	name := filepath.Base(path)
	rec := &CrashRecord{
		File:      path,
		Pid:       pidFromName(name),
		StartedAt: startedAtFromName(name),
		Reason:    extractReason(buf),
		Raw:       buf,
		Truncated: truncated,
	}
	return rec, nil
}

// extractReason 扫描首批行，找到 "fatal error:" 或 "panic:" 那一行返回。
// runtime 写 dump 的第一行通常就是 "fatal error: concurrent map writes"。
func extractReason(buf []byte) string {
	const maxScan = 4 * 1024
	end := len(buf)
	if end > maxScan {
		end = maxScan
	}
	head := string(buf[:end])
	for _, line := range strings.Split(head, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "fatal error:") || strings.HasPrefix(l, "panic:") {
			return l
		}
	}
	return ""
}

// defaultUploader 默认实现：把 CrashRecord.Render() 包成 error
// 走 notify.SendErrMessage，与 SafeFunc 的 panic 告警共用通道，
// 便于运维侧统一接收 / 聚合。
func defaultUploader(ctx context.Context, rec *CrashRecord) error {
	notify.SendErrMessage(ctx, errors.New(rec.Render()))
	return nil
}

// cleanupUploaded 删除 dir 下 mtime 早于 (now - keep) 的 *.uploaded 文件。
// 静默吞错：清理失败不影响主流程，下次再试即可。
func cleanupUploaded(dir string, keep time.Duration) {
	if keep <= 0 {
		return
	}
	cutoff := time.Now().Add(-keep)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), crashFileSuffix+uploadedSuffix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
