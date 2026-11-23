package grpc

import (
	"strconv"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Cotary/go-lib/common/defined"
	e "github.com/Cotary/go-lib/err"
)

// ============================================================================
// 还原
// ============================================================================

// ToBusinessError 从 gRPC 错误里取出业务错误码。
//
// 只在对端明确携带了业务码（ErrorInfo.metadata.biz_code）时返回非 nil：
//
//	if codeErr, ok := ToBusinessError(rawErr); ok {
//	    if errors.Is(codeErr, defined.OrderNotFound) { ... }
//	}
//
// 找不到返回 nil, false —— 不编造。对端是第三方服务、或返回的是纯协议错误
// （Unavailable / DeadlineExceeded）时都属此类，调用方应继续按 status code 判断。
func ToBusinessError(err error) (*e.CodeErr, bool) {
	if err == nil {
		return nil, false
	}
	st, ok := status.FromError(err)
	if !ok {
		return nil, false
	}
	return codeErrFromStatus(st)
}

// codeErrFromStatus 从 status 里取业务码。没有业务码时返回 nil, false。
func codeErrFromStatus(st *status.Status) (*e.CodeErr, bool) {
	_, meta := parseErrorInfo(st)

	// 业务码必须来自 metadata。没有就不是业务错误，直接返回 nil。
	//
	// 不能用协议状态码兜底：1~16 会和真实业务码撞车（status Canceled 的 1
	// 会匹配上 MissingCredentialsErr 的 1），errors.Is 出现误判。
	code, parseErr := strconv.Atoi(meta[defined.MDBizCode])
	if parseErr != nil {
		return nil, false
	}

	msg := st.Message()

	// 类别只从 status.code 判定，不看 reason。
	class := classOf(st.Code())

	return e.NewCodeErr(code, class, msg, e.InfoLevel), true
}

// ToGRPCStatus 尝试把错误还原成 gRPC status。
//
// 和 ToBusinessError 对称：一个取传输语义，一个取业务语义。
// 两者都只在「确实能取到」时返回非 nil，不拿默认值糊弄调用方。
//
//	st, ok := ToGRPCStatus(err)
//	if ok {
//	    log(st.Code(), st.Message())
//	}
func ToGRPCStatus(err error) (*status.Status, bool) {
	if err == nil {
		return nil, false
	}
	st, ok := status.FromError(err)
	if !ok {
		return nil, false
	}
	return st, true
}

// Reason 取出错误的类别常量（NOT_FOUND / UNAUTHENTICATED ...），细分靠 BizCode 区分。
func Reason(err error) string {
	st, ok := ToGRPCStatus(err)
	if !ok {
		return ""
	}
	reason, _ := parseErrorInfo(st)
	return reason
}

// BizCode 取出错误携带的业务码。
func BizCode(err error) int {
	st, ok := ToGRPCStatus(err)
	if !ok {
		return 0
	}
	_, meta := parseErrorInfo(st)
	code, _ := strconv.Atoi(meta[defined.MDBizCode])
	return code
}

// Domain 取出错误来源的服务名。
func Domain(err error) string {
	st, ok := ToGRPCStatus(err)
	if !ok {
		return ""
	}
	for _, detail := range st.Details() {
		if info, ok := detail.(*errdetails.ErrorInfo); ok {
			return info.Domain
		}
	}
	return ""
}

// FieldViolations 取出字段级校验错误。
func FieldViolations(err error) []e.FieldViolation {
	st, ok := ToGRPCStatus(err)
	if !ok {
		return nil
	}
	for _, detail := range st.Details() {
		bad, ok := detail.(*errdetails.BadRequest)
		if !ok {
			continue
		}
		out := make([]e.FieldViolation, 0, len(bad.FieldViolations))
		for _, fv := range bad.FieldViolations {
			out = append(out, e.FieldViolation{Field: fv.Field, Description: fv.Description})
		}
		return out
	}
	return nil
}

// parseErrorInfo 从 details 里取出 ErrorInfo 的 Reason 和 Metadata。
func parseErrorInfo(st *status.Status) (string, map[string]string) {
	meta := map[string]string{}
	for _, detail := range st.Details() {
		info, ok := detail.(*errdetails.ErrorInfo)
		if !ok {
			continue
		}
		for k, v := range info.Metadata {
			meta[k] = v
		}
		return info.Reason, meta
	}
	return "", meta
}

// classOf 把 status code 反向映射成错误类别。
//
// e.Class 的数值与 codes.Code 一一对应，直接强转；越界降级为 Unknown。
func classOf(c codes.Code) e.Class {
	if c > codes.Unauthenticated {
		return e.ClassUnknown
	}
	return e.Class(c)
}
