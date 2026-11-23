package grpc

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/log"
	"github.com/google/uuid"
)

// ============================================================================
// 请求 ID
// ============================================================================

// RequestIDInterceptor 生成或透传请求 ID，写进 outgoing metadata。
func RequestIDInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = withOutgoingRequestID(ctx)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// RequestIDStreamInterceptor 是 stream 版的 RequestIDInterceptor。
func RequestIDStreamInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx = withOutgoingRequestID(ctx)
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// withOutgoingRequestID 把请求 ID 写进 outgoing metadata。
func withOutgoingRequestID(ctx context.Context) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)

	for _, v := range md.Get(defined.MDRequestID) {
		if v != "" {
			return ctx
		}
	}

	md = md.Copy()
	md.Set(defined.MDRequestID, uuid.NewString())
	return metadata.NewOutgoingContext(ctx, md)
}

// ============================================================================
// 凭证
// ============================================================================

// AuthInterceptor 把凭证注入 outgoing metadata。
func AuthInterceptor(token string, key ...string) grpc.UnaryClientInterceptor {
	mdKey := defined.MDAuthorization
	if len(key) > 0 && key[0] != "" {
		mdKey = strings.ToLower(key[0])
	}
	value := token
	bearerPrefix := strings.ToLower(defined.BearerScheme) + " "
	if mdKey == defined.MDAuthorization && !strings.HasPrefix(strings.ToLower(token), bearerPrefix) {
		value = defined.BearerScheme + " " + token
	}
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, mdKey, value)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ============================================================================
// 日志
// ============================================================================

// LogInterceptor 记录每次调用的日志。
func LogInterceptor(logger *slog.Logger) grpc.UnaryClientInterceptor {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		cost := time.Since(start)

		fields := map[string]any{
			defined.LogKeyMethod: method,
			defined.LogKeyCostMS: cost.Milliseconds(),
			defined.LogKeyTarget: cc.Target(),
		}
		if err != nil {
			fields[defined.LogKeyCode] = status.Code(err).String()
			fields[defined.LogKeyError] = err.Error()
			logger.ErrorContext(ctx, defined.LogMsgGRPCCall, log.WithFields(fields)...)
		} else {
			fields[defined.LogKeyCode] = codes.OK.String()
			logger.InfoContext(ctx, defined.LogMsgGRPCCall, log.WithFields(fields)...)
		}
		return err
	}
}

// ============================================================================
// 重试
// ============================================================================

// RetryPolicy 是重试策略。
type RetryPolicy struct {
	// MaxRetries 最大重试次数。0 表示不重试。
	MaxRetries int
	// Backoff 首次退避时长，后续按倍数递增。
	Backoff time.Duration
	// MaxBackoff 单次退避上限。
	MaxBackoff time.Duration
	// Multiplier 退避倍数。默认 2。
	Multiplier float64
}

// DefaultRetryPolicy 是默认的重试策略。
var DefaultRetryPolicy = RetryPolicy{
	MaxRetries: 2,
	Backoff:    100 * time.Millisecond,
	MaxBackoff: 2 * time.Second,
	Multiplier: 2,
}

// RetryInterceptor 按状态码白名单重试（Unavailable / ResourceExhausted / Aborted）。
//
// 只对幂等操作挂载：非幂等的写操作重试可能写坏数据。
func RetryInterceptor(policy ...RetryPolicy) grpc.UnaryClientInterceptor {
	p := DefaultRetryPolicy
	if len(policy) > 0 {
		p = policy[0]
	}
	if p.Multiplier <= 0 {
		p.Multiplier = 2
	}
	if p.MaxBackoff <= 0 {
		p.MaxBackoff = 2 * time.Second
	}

	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var lastErr error
		backoff := p.Backoff

		for attempt := 0; attempt <= p.MaxRetries; attempt++ {
			err := invoker(ctx, method, req, reply, cc, opts...)
			if err == nil {
				return nil
			}
			lastErr = err

			if !IsRetryable(err) {
				return err
			}
			if attempt == p.MaxRetries {
				break
			}

			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}

			backoff = time.Duration(float64(backoff) * p.Multiplier)
			if backoff > p.MaxBackoff {
				backoff = p.MaxBackoff
			}
		}
		return lastErr
	}
}
