package handler

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// rawHandler 只输出 Record.Message + 换行，不做任何编码。
//
// 适合第三方文本（SQL、访问日志）：字段、时间、级别、调用点全部丢弃，
// 只保留消息原文。级别过滤仍然生效。
type rawHandler struct {
	mu    *sync.Mutex
	w     io.Writer
	level slog.Level
}

// NewRaw 创建原样输出的 Handler：每条记录只写 msg，末尾补一个换行。
func NewRaw(w io.Writer, level slog.Level) slog.Handler {
	return &rawHandler{mu: new(sync.Mutex), w: w, level: level}
}

func (h *rawHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level
}

func (h *rawHandler) Handle(_ context.Context, r slog.Record) error {
	msg := strings.TrimRight(r.Message, "\r\n")

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, msg+"\n")
	return err
}

// WithAttrs 原样输出不使用字段，直接返回自身。
func (h *rawHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

// WithGroup 原样输出不使用分组，直接返回自身。
func (h *rawHandler) WithGroup(string) slog.Handler { return h }
