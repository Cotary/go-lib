package grpcserver

import (
	"context"
	"strings"

	"google.golang.org/grpc/metadata"

	"github.com/Cotary/go-lib/common/defined"
)

// ============================================================================
// metadata key
// ============================================================================

// metadata key 全小写（gRPC 规范）。值来自 common/defined，与 net/grpc 同源，
// 避免客户端写、服务端读的两份字面量漂移。保留 MD* 名字不破坏既有引用。
const (
	MDRequestID     = defined.MDRequestID
	MDToken         = defined.MDToken
	MDAppid         = defined.MDAppid
	MDNonce         = defined.MDNonce
	MDSign          = defined.MDSign
	MDSignTimestamp = defined.MDSignTimestamp
	MDSignType      = defined.MDSignType
	MDAuthorization = defined.MDAuthorization
)

// ============================================================================
// 读取
// ============================================================================

// GetString 读取 incoming metadata 的一个值（key 大小写不敏感）。
func GetString(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	return firstValue(md, key)
}

// GetToken 读取调用方凭证。
//
// 优先读标准的 authorization 头（Bearer 形式），再回退到 x-token。
func GetToken(ctx context.Context) string {
	if v := GetString(ctx, MDAuthorization); v != "" {
		// 大小写不敏感地剥掉 Bearer 前缀。
		bearerPrefix := strings.ToLower(defined.BearerScheme) + " "
		if len(v) >= len(bearerPrefix) &&
			strings.ToLower(v[:len(bearerPrefix)]) == bearerPrefix {
			return strings.TrimSpace(v[len(bearerPrefix):])
		}
		return strings.TrimSpace(v)
	}
	return GetString(ctx, MDToken)
}

// GetRequestID 读取请求 ID。
//
// 先看 incoming metadata（上游透传过来的），再看 ctx value（本服务生成的）。
// 两者分开存：metadata 是线上的，ctx value 是进程内的；只读 metadata
// 会让 RequestIDInterceptor 生成的 ID 在日志里丢掉。
func GetRequestID(ctx context.Context) string {
	if id := GetString(ctx, MDRequestID); id != "" {
		return id
	}
	id, _ := ctx.Value(defined.RequestID).(string)
	return id
}

// ============================================================================
// 写入
// ============================================================================

// SetRequestID 把请求 ID 写入 ctx value。
//
// 写 ctx value 而不是 metadata：metadata 代表「收到的请求头」，不应被改写；
// 而下游日志、告警、gorm 慢 SQL 都从 ctx value 取（和 HTTP 侧一致）。
func SetRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, defined.RequestID, id)
}

// AppendToOutgoing 往 outgoing metadata 追加键值对。
func AppendToOutgoing(ctx context.Context, kv ...string) context.Context {
	if len(kv) < 2 {
		return ctx
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	for i := 0; i+1 < len(kv); i += 2 {
		md.Append(strings.ToLower(kv[i]), kv[i+1])
	}
	return metadata.NewOutgoingContext(ctx, md)
}

// CopyIncomingToOutgoing 把 incoming metadata 全量复制到 outgoing，用于向调用下游透传链路信息。
//
// ⚠️ 凭证也会一起复制：authorization / x-token 会随请求发给下游。
// "用用户身份调下游" 时这是想要的；只想要链路 ID 时不要用它，
// 改用 AppendToOutgoing 挑着传。
func CopyIncomingToOutgoing(ctx context.Context) context.Context {
	in, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	for k, vs := range in {
		for _, v := range vs {
			md.Append(k, v)
		}
	}
	return metadata.NewOutgoingContext(ctx, md)
}

// ============================================================================
// 内部
// ============================================================================

// firstValue 取第一个非空值。metadata.MD.Get 内部已统一小写，无需再兜底。
func firstValue(md metadata.MD, key string) string {
	for _, v := range md.Get(key) {
		if v != "" {
			return v
		}
	}
	return ""
}
