package grpcserver

import (
	"context"
	"encoding/json"
	"strconv"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Cotary/go-lib/common/appctx"
	"github.com/Cotary/go-lib/common/defined"
	e "github.com/Cotary/go-lib/err"
	"github.com/Cotary/go-lib/notify"
)

// ============================================================================
// 错误收口
// ============================================================================

// GRPCErrHandler 是 gRPC 侧的错误收口点：
//
//  1. 归一化错误 → 业务错误
//  2. 按级别决定是否告警
//  3. 映射成 status（code + message + details）
//
// 业务层只声明错误语义，不碰这里。
func GRPCErrHandler(ctx context.Context, err error) *status.Status {
	bizErr := e.AsBusinessErr(err)
	if bizErr == nil {
		bizErr = e.NewBusinessErr(e.FailedErr, nil)
	}

	// ★ 告警判定只在这里，和 HTTP 侧同一套逻辑、同一套阈值。
	if bizErr.Level <= e.WarnLevel {
		notify.SendErrMessage(ctx, err)
	}

	code := codeOf(bizErr)
	msg := bizErr.Msg
	// 测试环境把底层错误拼进 message，方便排查。
	if appctx.OrDefault(ctx).Env == defined.TEST && bizErr.Err != nil && bizErr.Err.Error() != "" {
		msg = bizErr.Err.Error()
	}

	st := status.New(code, msg)
	return attachDetails(ctx, st, bizErr)
}

// codeOf 决定 status code：只看 Class，不看 bizErr.Err。
//
// 【为什么不透传底层 status】BusinessErr.Err 的契约是「内部用，不发给客户端」。
// 从它取 code 会让 code、reason、message、biz_code 四个字段分成两个来源：
//
//	bizErr := e.NewBusinessErr(e.FailedErr, status.Error(codes.NotFound, "..."))
//	→ code=NotFound（来自 Err）、reason="INTERNAL"（来自 Class）→ 自相矛盾
//
// 也确实不能无条件采用底层 status：业务可能是在做错误转译
// （把下游 Unavailable 收敛成本服务对外的 FailedErr），覆盖它会改变
// 客户端的重试决策。所以统一以 Class 为准。
//
// 需要原样透传下游 status 时，直接 return 那个 status 错误即可——
// ErrorUnaryInterceptor 里已有放行判断。
func codeOf(bizErr *e.BusinessErr) codes.Code {
	// e.Class 的数值与 codes.Code 一一对应（见 err/codeErr.go 的常量），
	// 直接强转即可，不必手写 17 个 case——那样子类一多就会漏。
	if bizErr.Class > e.ClassUnauthenticated {
		return codes.Internal
	}
	return codes.Code(bizErr.Class)
}

// attachDetails 挂载错误详情：ErrorInfo（必带）+ BadRequest（字段级校验，可选）。
func attachDetails(ctx context.Context, st *status.Status, bizErr *e.BusinessErr) *status.Status {
	// reason 用类别常量；metadata 只放业务码，不污染 reason。
	//
	// 文案只走 status.message，不再在 metadata 里重复一份：
	// 同一句话传两遍一旦不一致（比如 TEST 环境 message 拼了底层错误），
	// 客户端该信哪个就成了问题。
	meta := map[string]string{
		defined.MDBizCode: strconv.Itoa(bizErr.Code),
	}

	// ---- 字段级校验错误：google.rpc.BadRequest ----
	var violations *errdetails.BadRequest
	switch data := bizErr.Data.(type) {
	case []e.FieldViolation:
		violations = &errdetails.BadRequest{}
		for _, fv := range data {
			violations.FieldViolations = append(violations.FieldViolations,
				&errdetails.BadRequest_FieldViolation{
					Field:       fv.Field,
					Description: fv.Description,
				})
		}
	case string:
		meta[defined.MDData] = data
	case nil:
		// 无附加数据
	default:
		if raw, err := json.Marshal(data); err == nil {
			meta[defined.MDData] = string(raw)
		}
	}

	// reason 放业务文案：客户端只认 status.code 判类别，reason 仅用于人读与日志检索。
	//
	// 这偏离了 ErrorInfo.reason 的 UPPER_SNAKE_CASE 约定（AIP-193），是有意取舍：
	// 把文案放这里，排查时直接看得懂，不必再翻 biz_code 对照表。
	info := &errdetails.ErrorInfo{
		Reason:   bizErr.Msg,
		Domain:   domainOf(ctx),
		Metadata: meta,
	}
	if newSt, err := st.WithDetails(info); err == nil {
		st = newSt
	}

	if violations != nil {
		if newSt, err := st.WithDetails(violations); err == nil {
			st = newSt
		}
	}

	return st
}

// domainOf 返回 error 的 domain，即服务名。
//
// 从 ctx 取而非全局默认值：多实例/多租户下同一进程可能持有不同身份，
// 读全局会串。缺失时填 "unknown"，不拿库名充数。
func domainOf(ctx context.Context) string {
	if name := appctx.OrDefault(ctx).Service; name != "" {
		return name
	}
	return defined.UnknownGRPCDomain
}

// FromError 把业务错误转成可直接返回给 gRPC 的 error。
func FromError(ctx context.Context, err error) error {
	return GRPCErrHandler(ctx, err).Err()
}
