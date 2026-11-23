package grpc

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Code 取出错误的 gRPC 状态码。
//
// 非 gRPC 错误返回 codes.Unknown。
func Code(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	st, ok := ToGRPCStatus(err)
	if !ok {
		return codes.Unknown
	}
	return st.Code()
}

// IsRetryable 判断错误是否值得退避重试。
//
// 【判据来自 gRPC 的语义】
//
//	Unavailable         服务暂时不可用，退避后重试可能成功
//	ResourceExhausted   配额/限流，退避后重试可能成功
//	Aborted             并发冲突，重试可能成功
//
// 其余一律不重试：InvalidArgument 重试还是错，
// Internal 重试多半还是错，重试非幂等操作还可能写坏数据。
func IsRetryable(err error) bool {
	switch Code(err) {
	case codes.Unavailable, codes.ResourceExhausted, codes.Aborted:
		return true
	default:
		return false
	}
}

// Details 取出错误附带的结构化详情。
func Details(err error) []any {
	st, ok := ToGRPCStatus(err)
	if !ok {
		return nil
	}
	return st.Details()
}

// IsNotFound 判断错误是否「资源不存在」。
//
// 【为什么需要】gRPC 里找不到资源有两种表达：NotFound 状态码，
// 或业务错误（订单不存在）。这个函数把两者都算上。
func IsNotFound(err error) bool {
	return Code(err) == codes.NotFound
}

// IsUnauthenticated 判断错误是否需要认证。
//
// 「没登录」「token 过期」「签名错误」都属于这一类，
// 细分靠 biz_code（grpcx.BizCode）或业务码判定。
func IsUnauthenticated(err error) bool {
	return Code(err) == codes.Unauthenticated
}

// 哨兵错误的文案。提为常量，避免同一句话在多处出现时手滑改错。
const (
	errMsgNotFound        = "not found"
	errMsgUnauthenticated = "unauthenticated"
	errMsgUnavailable     = "service unavailable"
)

// ErrNotFound 是一个标准的 NotFound 错误。
var ErrNotFound = status.Error(codes.NotFound, errMsgNotFound)

// ErrUnauthenticated 是一个标准的 Unauthenticated 错误。
var ErrUnauthenticated = status.Error(codes.Unauthenticated, errMsgUnauthenticated)

// ErrUnavailable 是一个标准的 Unavailable 错误。
var ErrUnavailable = status.Error(codes.Unavailable, errMsgUnavailable)

// IsStatus 判断错误是否是 gRPC status 错误。
//
// 等价于 ToGRPCStatus 的 ok 返回值，保留这个函数是为了让调用点读起来更直白。
func IsStatus(err error) bool {
	_, ok := ToGRPCStatus(err)
	return ok
}
