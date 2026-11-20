package handler

import (
	"context"
	"log/slog"
	"time"
)

// AlertKey 是"这条日志要告警"的标记字段名，不会出现在输出里。
const AlertKey = "_alert"

// Alert 是投递给 Alerter 的一条告警。
//
// Title / Msg 保留给抑制器做去重键（notify.Suppressor 依赖 Fingerprint）；
// Attrs 是日志里的结构化字段，已经过脱敏。
type Alert struct {
	Title string
	Msg   string
	Level slog.Level
	Time  time.Time
	Attrs []slog.Attr
}

// Fingerprint 返回稳定的去重键：同一条消息在不同时间/不同请求里算同一类告警。
func (a Alert) Fingerprint() string { return a.Title + "|" + a.Msg }

// Alerter 是告警出口。返回 error 时由 alertHandler 记一条错误日志，不重试。
type Alerter interface {
	Send(ctx context.Context, a Alert) error
}

// alertHandler 在记录带 AlertKey 标记时把告警转给 Alerter，
// 并保证标记本身不会写进日志。
type alertHandler struct {
	next    slog.Handler
	alerter Alerter
	flagged bool // With 阶段已经打上标记
	attrs   []slog.Attr
}

func (h *alertHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *alertHandler) Handle(ctx context.Context, r slog.Record) error {
	if !h.flagged && !recordHasKey(r, AlertKey) {
		return h.next.Handle(ctx, r)
	}

	if h.alerter != nil {
		if err := h.alerter.Send(ctx, h.toAlert(r)); err != nil {
			// 用 next 记错误，避免递归回 alertHandler。
			slog.New(h.next).ErrorContext(ctx, "alert send failed", "err", err)
		}
	}

	return h.next.Handle(ctx, withoutAttr(r, AlertKey))
}

func (h *alertHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	flagged := h.flagged
	kept := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		if a.Key == AlertKey {
			if a.Value.Kind() == slog.KindBool && a.Value.Bool() {
				flagged = true
			}
			continue
		}
		kept = append(kept, a)
	}

	merged := make([]slog.Attr, 0, len(h.attrs)+len(kept))
	merged = append(merged, h.attrs...)
	merged = append(merged, kept...)

	return &alertHandler{
		next:    h.next.WithAttrs(kept),
		alerter: h.alerter,
		flagged: flagged,
		attrs:   merged,
	}
}

func (h *alertHandler) WithGroup(name string) slog.Handler {
	return &alertHandler{next: h.next.WithGroup(name), alerter: h.alerter, flagged: h.flagged, attrs: h.attrs}
}

func (h *alertHandler) toAlert(r slog.Record) Alert {
	attrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key != AlertKey {
			attrs = append(attrs, a)
		}
		return true
	})
	return Alert{
		Title: r.Message,
		Msg:   r.Message,
		Level: r.Level,
		Time:  r.Time,
		Attrs: attrs,
	}
}
