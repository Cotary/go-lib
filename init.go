package lib

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/Cotary/go-lib/common/appctx"
	"github.com/Cotary/go-lib/common/coroutines"
	"github.com/Cotary/go-lib/log"
	"github.com/Cotary/go-lib/provider/message"
)

func Init(serverName, env string) {
	appctx.Init(serverName, env)
}

func InitLog(logger log.Logger) {
	log.SetGlobalLogger(logger)
}

func InitGlobalSender(sender message.Sender) {
	message.SetGlobalSender(sender)
}

// InitCrashReporter 启动进程级 fatal error 兜底捕获。
//
// 调用时机：必须放在最早期，至少在任何业务 goroutine 启动之前；
// 否则在注册之前发生的 fatal 仍只会落到 stderr。详细能力边界
// 与配置项见 common/coroutines/crash.go 文档。
//
// 多数业务建议直接用 BootstrapCrashCapture，省去手动两阶段调用。
func InitCrashReporter(opts ...coroutines.CrashOption) error {
	return coroutines.InitCrashReporter(opts...)
}

// ReportPendingCrashes 扫描历史 crash dump 文件并通过 notify 通道补报。
//
// 调用时机：必须在 InitGlobalSender 完成后调用，否则默认 uploader
// 取不到 sender，告警会丢。一次性调用即可，不需要常驻协程。
func ReportPendingCrashes(ctx context.Context) error {
	return coroutines.ReportPendingCrashes(ctx)
}

// BootstrapCrashCapture 是面向"每个项目零定制接入"的一键 helper，
// 把崩溃捕获的两阶段调用合二为一（实际上仍是两阶段，但写法更直白）：
//
//  1. 立即调用 InitCrashReporter，注册 SetCrashOutput；
//     默认目录是 ./logs/crash/<serverName>，能避免同主机多服务相互干扰。
//     若 appctx 还没初始化拿不到 serverName，则退回 ./logs/crash。
//  2. 返回一个 flush 闭包，业务在 InitGlobalSender 之后调用即可补报。
//
// 行为约定：
//   - 任何错误都被静默吞掉，不影响业务启动（容器无写权限场景需留意）；
//   - 即使 InitCrashReporter 失败，flush 闭包也安全可调（内部为 nop）；
//   - 重复调用幂等（依托 InitCrashReporter 的幂等保护）。
//
// 推荐模板（每个项目 main 直接复制）：
//
//	lib.Init("my-svc", "prod")
//	lib.InitLog(logger)
//
//	flush := lib.BootstrapCrashCapture()
//	// 业务可继续追加自定义选项，例如：
//	// flush := lib.BootstrapCrashCapture(coroutines.WithCrashDir("/data/crash"))
//
//	lib.InitGlobalSender(sender)
//	flush(coroutines.NewContext("crash-report"))
//
// 如需更精细控制（自定义 uploader、限定 dump 大小等），把 With* 选项
// 当参数传进来即可，与裸调 InitCrashReporter 完全等价。
func BootstrapCrashCapture(opts ...coroutines.CrashOption) func(ctx context.Context) {
	merged := append([]coroutines.CrashOption{coroutines.WithCrashDir(defaultCrashDir())}, opts...)
	if err := coroutines.InitCrashReporter(merged...); err != nil {
		log.WithContext(context.Background()).
			WithField("action", "BootstrapCrashCapture").
			Error(err.Error())
		return func(context.Context) {}
	}
	return func(ctx context.Context) {
		if err := coroutines.ReportPendingCrashes(ctx); err != nil {
			log.WithContext(ctx).
				WithField("action", "ReportPendingCrashes").
				Error(err.Error())
		}
	}
}

// defaultCrashDir 优先按 serverName 隔离，避免同主机多服务共用目录时
// "B 进程启动扫到 A 进程还在写的文件"。serverName 为空时退回根目录。
//
// 使用 filepath.Join 而非字符串拼接，保证跨平台路径分隔符正确。
// 对 serverName 做基本清洗（去掉 / \ 等路径字符），避免被恶意构造目录穿越。
func defaultCrashDir() string {
	name := strings.TrimSpace(appctx.ServerName())
	if name == "" {
		return "./logs/crash"
	}
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, name)
	return filepath.Join("./logs/crash", cleaned)
}
