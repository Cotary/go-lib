package message

import (
	"context"
	"sync/atomic"

	"github.com/Cotary/go-lib/common/utils"
)

// noopSender 是不做任何事、也不返回错误的默认出口。
type noopSender struct{}

func (noopSender) Send(context.Context, string, *utils.OrderedMap[string, string]) error {
	return nil
}

// defaultSender 参考 slog 的实现：进程启动即有一个可用实例，
// Default 永远返回非 nil，调用方不需要判空。
var defaultSender atomic.Pointer[Sender]

func init() {
	s := Sender(noopSender{})
	defaultSender.Store(&s)
}

// SetDefault 设置进程默认的消息出口，用法与 slog.SetDefault 一致。
//
// 传 nil 等价于恢复为 no-op 出口。
func SetDefault(s Sender) {
	if s == nil {
		s = noopSender{}
	}
	defaultSender.Store(&s)
}

// Default 返回进程默认的消息出口，永远非 nil。
func Default() Sender {
	if p := defaultSender.Load(); p != nil {
		return *p
	}
	return noopSender{}
}

// SetGlobalSender 是 SetDefault 的旧名字。
//
// Deprecated: 使用 SetDefault。
func SetGlobalSender(s Sender) { SetDefault(s) }

// GetGlobalSender 是 Default 的旧名字。
//
// Deprecated: 使用 Default。
func GetGlobalSender() Sender { return Default() }
