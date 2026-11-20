package appctx

import (
	"context"
	"sync"

	"github.com/Cotary/go-lib/common/defined"
)

// Identity 是进程/请求的身份信息。
type Identity struct {
	Service string
	Env     string
}

// IsZero 判断是否为空身份。
func (id Identity) IsZero() bool { return id.Service == "" && id.Env == "" }

// WithContext 把身份写入 ctx。
func (id Identity) WithContext(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, defined.ServerName, id.Service)
	ctx = context.WithValue(ctx, defined.ENV, id.Env)
	return ctx
}

// FromContext 从 ctx 读取身份；读不到返回零值。
func FromContext(ctx context.Context) Identity {
	var id Identity
	if ctx == nil {
		return id
	}
	id.Service, _ = ctx.Value(defined.ServerName).(string)
	id.Env, _ = ctx.Value(defined.ENV).(string)
	return id
}

// OrDefault 优先从 ctx 读取，读不到时回退到进程默认身份。
func OrDefault(ctx context.Context) Identity {
	if id := FromContext(ctx); !id.IsZero() {
		return id
	}
	return Default()
}

var (
	defaultIdentity Identity
	mu              sync.RWMutex
)

// SetDefault 设置进程默认身份，用法与 slog.SetDefault 一致。
func SetDefault(id Identity) {
	mu.Lock()
	defer mu.Unlock()
	defaultIdentity = id
}

// Default 返回进程默认身份。
func Default() Identity {
	mu.RLock()
	defer mu.RUnlock()
	return defaultIdentity
}
