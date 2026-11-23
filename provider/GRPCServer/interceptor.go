package grpcserver

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Cotary/go-lib/common/appctx"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/log"
	"github.com/Cotary/go-lib/notify"
	"github.com/google/uuid"
)

// ============================================================================
// panic 恢复
// ============================================================================

// RecoveryUnaryInterceptor 捕获 unary 的 panic。
//
// 堆栈只进日志：message 对客户端可见，不能带现场。
func RecoveryUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = handlePanic(ctx, info.FullMethod, r)
			}
		}()
		return handler(ctx, req)
	}
}

// RecoveryStreamInterceptor 捕获 stream 处理过程中的 panic。
func RecoveryStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = handlePanic(ss.Context(), info.FullMethod, r)
			}
		}()
		return handler(srv, ss)
	}
}

// handlePanic 把 panic 转成 SystemErr 并记录完整现场。
//
// 走收口而不是裸 status：客户端要能拿到 biz_code。
func handlePanic(ctx context.Context, method string, r any) error {
	stack := string(debug.Stack())

	slog.Default().ErrorContext(ctx, defined.LogMsgGRPCPanicRecovered,
		defined.LogKeyMethod, method, defined.LogKeyPanic, r, defined.LogKeyStack, stack)
	notify.SendErrMessage(ctx, fmt.Errorf("gRPC panic recovered in %s: %v", method, r))

	// 走收口而不是裸 status：客户端要拿到 biz_code 才能区分错误类型。
	return GRPCErrHandler(ctx, e.NewBusinessErr(e.SystemErr, fmt.Errorf("panic: %v", r))).Err()
}

// ============================================================================
// 错误收口
// ============================================================================

// ErrorUnaryInterceptor 把业务错误映射成 gRPC status。
//
// 没有它，业务方法返回的普通 error 会被框架包成 codes.Unknown，
// 业务码和 details 全丢，客户端 errors.Is 失效。
//
// 必须最后注册（最内层）：放在外层不会改变客户端结果（下面有放行判断），
// 但日志会看到未映射的 code=Unknown。
func ErrorUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}
		// 已是 status 的错误原样放行。
		//
		// 不能用 status.FromError：它走 errors.As，会把「包装了 status 的
		// 业务错误」也判成 status 而跳过映射。这里只认直接实现 GRPCStatus 的。
		if isStatusErr(err) {
			return resp, err
		}
		return resp, GRPCErrHandler(ctx, err).Err()
	}
}

// ErrorStreamInterceptor 是 stream 版的 ErrorUnaryInterceptor。
//
// 同样必须最后注册。stream 的业务错误若不映射，会被框架包成 codes.Unknown。
func ErrorStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		err := handler(srv, ss)
		if err == nil {
			return nil
		}
		if isStatusErr(err) {
			return err
		}
		return GRPCErrHandler(ss.Context(), err).Err()
	}
}

// isStatusErr 判断错误本身是否已是 gRPC status。
//
// 不能用 status.FromError：它走 errors.As，会把「包装了 status 的业务错误」
// 也判成 status 而跳过映射退化成 Unknown 之外的错误。这里只认直接实现
// GRPCStatus 的（即 status.Error 的返回值）。
func isStatusErr(err error) bool {
	_, ok := err.(interface{ GRPCStatus() *status.Status })
	return ok
}

// ============================================================================
// 请求 ID
// ============================================================================

// RequestIDInterceptor 生成或透传请求 ID。
//
// 优先取 incoming metadata 里的 x-request-id（上游传了就沿用），
// 没有就生成一个。写进 ctx 供下游日志使用，并回写到 response header。
func RequestIDInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, _ = withRequestID(ctx, info.FullMethod)
		return handler(ctx, req)
	}
}

// RequestIDStreamInterceptor 是 stream 版的 RequestIDInterceptor。
func RequestIDStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, _ := withRequestID(ss.Context(), info.FullMethod)
		return handler(srv, &serverStreamWithContext{ServerStream: ss, ctx: ctx})
	}
}

// withRequestID 生成请求 ID 并写进 ctx。
func withRequestID(ctx context.Context, method string) (context.Context, string) {
	id := GetRequestID(ctx)
	if id == "" {
		id = uuid.NewString()
	}
	ctx = SetRequestID(ctx, id)
	// 回写 header，方便客户端拿到同一个 ID 做链路追踪。
	_ = grpc.SetHeader(ctx, metadataWith(MDRequestID, id))
	return ctx, id
}

// ============================================================================
// 日志
// ============================================================================

// logConfig 日志拦截器的配置。
type logConfig struct {
	// metadataKeys 允许记录的 metadata key 白名单。
	metadataKeys []string
	// slowThreshold 慢调用阈值，超过则升级为 Warn。
	slowThreshold time.Duration
}

// LogOption 日志拦截器的配置项。
type LogOption func(*logConfig)

// 默认记录的 metadata key：只有排查价值，且不含敏感信息。
var defaultLogMetadataKeys = []string{defined.MDContentType, defined.MDUserAgent}

// 强制不记录的 metadata key：无论白名单怎么配都不记。
var sensitiveMetadataKeys = map[string]bool{
	defined.MDAuthorization: true,
	defined.MDToken:         true,
	defined.MDCookie:        true,
	defined.MDSign:          true,
	defined.MDAppid:         true,
}

// WithLogMetadataKeys 设置允许记录的 metadata key。
//
// 敏感 key（authorization、x-token 等）永远不会被记录。
func WithLogMetadataKeys(keys []string) LogOption {
	return func(c *logConfig) {
		c.metadataKeys = keys
	}
}

// WithLogSlowThreshold 设置慢调用阈值，超过则日志升级为 Warn。
func WithLogSlowThreshold(d time.Duration) LogOption {
	return func(c *logConfig) { c.slowThreshold = d }
}

// LogUnaryInterceptor 记录 unary 调用日志。默认只记元信息，不记消息体。
func LogUnaryInterceptor(logger *slog.Logger, opts ...LogOption) grpc.UnaryServerInterceptor {
	cfg := &logConfig{metadataKeys: defaultLogMetadataKeys}
	for _, opt := range opts {
		opt(cfg)
	}
	if logger == nil {
		logger = slog.Default()
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		start := time.Now()
		resp, err = handler(ctx, req)
		cost := time.Since(start)

		fields := logFields(ctx, info.FullMethod, err, cost, cfg)
		if err != nil {
			logger.ErrorContext(ctx, defined.LogMsgGRPCRequest, log.WithFields(fields)...)
		} else if cfg.slowThreshold > 0 && cost >= cfg.slowThreshold {
			logger.WarnContext(ctx, defined.LogMsgGRPCRequestSlow, log.WithFields(fields)...)
		} else {
			logger.InfoContext(ctx, defined.LogMsgGRPCRequest, log.WithFields(fields)...)
		}
		return resp, err
	}
}

// LogStreamInterceptor 是 stream 版的 LogUnaryInterceptor。
func LogStreamInterceptor(logger *slog.Logger, opts ...LogOption) grpc.StreamServerInterceptor {
	cfg := &logConfig{metadataKeys: defaultLogMetadataKeys}
	for _, opt := range opts {
		opt(cfg)
	}
	if logger == nil {
		logger = slog.Default()
	}

	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		cost := time.Since(start)

		fields := logFields(ss.Context(), info.FullMethod, err, cost, cfg)
		if err != nil {
			logger.ErrorContext(ss.Context(), defined.LogMsgGRPCStream, log.WithFields(fields)...)
		} else {
			logger.InfoContext(ss.Context(), defined.LogMsgGRPCStream, log.WithFields(fields)...)
		}
		return err
	}
}

// logFields 组装日志字段。
func logFields(ctx context.Context, method string, err error, cost time.Duration, cfg *logConfig) map[string]any {
	fields := map[string]any{
		defined.LogKeyMethod: method,
		defined.LogKeyCostMS: cost.Milliseconds(),
		defined.RequestID:    GetRequestID(ctx),
		defined.LogKeyServer: appctx.OrDefault(ctx).Service,
	}

	if err != nil {
		fields[defined.LogKeyCode] = status.Code(err).String()
		fields[defined.LogKeyError] = err.Error()
	} else {
		fields[defined.LogKeyCode] = codes.OK.String()
	}

	if md := filterMetadata(ctx, cfg.metadataKeys); len(md) > 0 {
		fields[defined.LogKeyMetadata] = md
	}
	return fields
}

// ============================================================================
// 鉴权
// ============================================================================

// Verifier 校验调用方凭证。
//
// 返回 nil 表示通过；返回错误则拒绝请求。
type Verifier func(ctx context.Context, token string) error

// AuthInterceptor 校验调用方凭证（从 metadata 取，不是 header）。
func AuthInterceptor(verifier Verifier) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := verify(ctx, verifier); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// AuthStreamInterceptor 是 stream 版的 AuthInterceptor。
func AuthStreamInterceptor(verifier Verifier) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := verify(ss.Context(), verifier); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

// verify 执行凭证校验。
//
// 【为什么这里要自己调收口】AuthInterceptor 注册在 ErrorUnaryInterceptor
// 外层，拦截器链是「外层先执行」，Auth 返回的错误不会再经过内层收口。
// 不自己调 GRPCErrHandler，就会丢掉告警（实测 PanicLevel 也不发）。
//
// 凭证类错误（AuthErr / SignErr / MissingCredentialsErr）本身已声明 ClassUnauthenticated，
// 走收口自然得到 Unauthenticated；verifier 返回别的类别时如实表达，不强行压平。
func verify(ctx context.Context, verifier Verifier) error {
	if verifier == nil {
		return nil
	}
	token := GetToken(ctx)
	if token == "" {
		// 走业务错误而不是裸 status：否则没有 biz_code，
		// 客户端 errors.Is(err, e.MissingCredentialsErr) 匹配不上。
		return GRPCErrHandler(ctx, e.NewBusinessErr(e.MissingCredentialsErr, nil)).Err()
	}
	if err := verifier(ctx, token); err != nil {
		return GRPCErrHandler(ctx, err).Err()
	}
	return nil
}

// ============================================================================
// 限流
// ============================================================================

// RateLimitInterceptor 按方法分组限流。
//
// keyFunc 决定限流分组键，默认按完整方法名。
// 触发限流返回 ResourceExhausted（8），客户端知道该退避重试。
func RateLimitInterceptor(limiter *utils.RateLimiter, keyFunc ...func(fullMethod string) string) grpc.UnaryServerInterceptor {
	keyOf := func(fullMethod string) string { return fullMethod }
	if len(keyFunc) > 0 && keyFunc[0] != nil {
		keyOf = keyFunc[0]
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if limiter != nil {
			if err := limiter.WaitKey(ctx, keyOf(info.FullMethod)); err != nil {
				return nil, GRPCErrHandler(ctx, e.NewBusinessErr(e.RequestLimitExceeded, err)).Err()
			}
		}
		return handler(ctx, req)
	}
}

// ============================================================================
// 超时
// ============================================================================

// TimeoutInterceptor 给没有 deadline 的调用兜一个超时。
//
// gRPC 原生支持 deadline，客户端没设时由服务端兜底，
// 避免请求无限期占用资源。
func TimeoutInterceptor(d time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if _, ok := ctx.Deadline(); ok {
			return handler(ctx, req)
		}
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()

		resp, err := handler(ctx, req)
		if err != nil && ctx.Err() == context.DeadlineExceeded {
			return resp, GRPCErrHandler(ctx, e.NewBusinessErr(e.TimeoutErr, err)).Err()
		}
		return resp, err
	}
}

// ============================================================================
// 参数校验
// ============================================================================

// Validator 是 protoc-gen-validate 生成的校验接口。
type Validator interface {
	Validate() error
}

// ValidateInterceptor 调 req.Validate()，失败走收口（ParamErr / InvalidArgument）。
func ValidateInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if v, ok := req.(Validator); ok {
			if err := v.Validate(); err != nil {
				return nil, GRPCErrHandler(ctx, e.NewBusinessErr(e.ParamErr, err)).Err()
			}
		}
		return handler(ctx, req)
	}
}

// ============================================================================
// 分组挂载
// ============================================================================

// WithMethodPrefix 让一组拦截器只对指定前缀的方法生效。
//
// grpc-go 的拦截器是扁平的，这个函数补上 HTTP r.Group 那种分组语义。
//
//	base := grpcserver.WithUnaryInterceptor(grpcserver.RequestIDInterceptor())
//	base = grpcserver.WithMethodPrefix(base, "/order.v1.OrderService/",
//	    grpcserver.AuthInterceptor(verifier))
func WithMethodPrefix(base []grpc.UnaryServerInterceptor, prefix string, extra ...grpc.UnaryServerInterceptor) []grpc.UnaryServerInterceptor {
	if len(extra) == 0 {
		return base
	}
	prefixed := make([]grpc.UnaryServerInterceptor, 0, len(base)+len(extra))
	prefixed = append(prefixed, base...)
	prefixed = append(prefixed, prefixInterceptor(prefix, extra...))
	return prefixed
}

// prefixInterceptor 是只对特定前缀生效的拦截器。
func prefixInterceptor(prefix string, is ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !strings.HasPrefix(info.FullMethod, prefix) {
			return handler(ctx, req)
		}
		// 依次执行这组拦截器，最后才到真正的 handler。
		next := handler
		for i := len(is) - 1; i >= 0; i-- {
			cur := is[i]
			inner := next
			next = func(ctx context.Context, req any) (any, error) {
				return cur(ctx, req, info, inner)
			}
		}
		return next(ctx, req)
	}
}
