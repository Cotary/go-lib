package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Cotary/go-lib/common/appctx"
	"github.com/Cotary/go-lib/common/defined"
	e "github.com/Cotary/go-lib/err"
)

// ===== 错误码 → status code 映射 =====

// TestCodeOf_BuiltinErrors: 每个内置错误的 Class 映射到正确的 codes.Code
func TestCodeOf_BuiltinErrors(t *testing.T) {
	cases := []struct {
		name string
		err  *e.CodeErr
		want codes.Code
	}{
		{"Param", e.ParamErr, codes.InvalidArgument},
		{"Timeout", e.TimeoutErr, codes.DeadlineExceeded},
		{"DataNotExist", e.DataNotExist, codes.NotFound},
		{"DataExist", e.DataExist, codes.AlreadyExists},
		{"Auth", e.AuthErr, codes.Unauthenticated},
		{"MissingCredentials", e.MissingCredentialsErr, codes.Unauthenticated},
		{"Verify", e.VerifyErr, codes.PermissionDenied},
		{"Permission", e.PermissionErr, codes.PermissionDenied},
		{"ServiceBusy", e.ServiceBusy, codes.ResourceExhausted},
		{"Limit", e.RequestLimitExceeded, codes.ResourceExhausted},
		{"Config", e.ConfigErr, codes.FailedPrecondition},
		{"Failed", e.FailedErr, codes.Internal},
		{"System", e.SystemErr, codes.Internal},
		{"Custom", e.CustomErr, codes.Unknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bizErr := e.NewBusinessErr(c.err, nil)
			if got := codeOf(bizErr); got != c.want {
				t.Errorf("codeOf = %v; want %v", got, c.want)
			}
		})
	}
}

// TestGRPCErrHandler_ClassToStatusCode: 收口点把 Class 映射成 status code
func TestGRPCErrHandler_ClassToStatusCode(t *testing.T) {
	st := GRPCErrHandler(context.Background(), e.NewBusinessErr(e.DataNotExist, nil))
	if st.Code() != codes.NotFound {
		t.Errorf("Code = %v; want %v", st.Code(), codes.NotFound)
	}
}

// ===== 业务码不进 status code，只进 details =====

// TestGRPCErrHandler_BizCodeNotStatusCode: 业务码不进 status code，只进 metadata
//
// 【关键设计】status code 只有 17 个取值，是「类别」；
// 业务码要能区分同类别的不同错误，必须走 details。
func TestGRPCErrHandler_BizCodeNotStatusCode(t *testing.T) {
	const bizCode = 20001
	bizErr := e.NewCodeErr(bizCode, e.ClassNotFound, "Order not found", e.InfoLevel)

	st := GRPCErrHandler(context.Background(), e.NewBusinessErr(bizErr, nil))

	if int(st.Code()) != int(e.ClassNotFound) {
		t.Errorf("status code = %v; want %v (class value)", st.Code(), e.ClassNotFound)
	}

	info := errorInfoOf(t, st)
	if got := info.Metadata[defined.MDBizCode]; got != strconv.Itoa(bizCode) {
		t.Errorf("%s = %q; want %q", defined.MDBizCode, got, strconv.Itoa(bizCode))
	}
	// 文案只走 status.message，details 里不重复
	if got, has := info.Metadata["msg"]; has {
		t.Errorf(`metadata["msg"] = %q; details 里不应再带文案`, got)
	}
}

// ===== reason 用业务文案 =====

// TestGRPCErrHandler_ReasonIsMessage: reason 放业务文案，方便排查时直接读懂。
//
// 【为什么不是类别常量】类别已经由 status.code 表达（17 大类），
// reason 再放一遍是同一信息的两种写法，且客户端会因两者不一致而出错。
func TestGRPCErrHandler_ReasonIsMessage(t *testing.T) {
	st := GRPCErrHandler(context.Background(), e.NewBusinessErr(e.DataNotExist, nil))

	info := errorInfoOf(t, st)
	if info.Reason != e.DataNotExist.Msg {
		t.Errorf("Reason = %q; want %q（应与业务文案一致）", info.Reason, e.DataNotExist.Msg)
	}
}

// TestGRPCErrHandler_ReasonDistinguishes: 同类别的不同错误，reason 也不同。
//
// 「没登录」和「签名过期」都是 UNAUTHENTICATED(16)，
// status.code 相同，靠 reason 文案和 biz_code 区分。
func TestGRPCErrHandler_ReasonDistinguishes(t *testing.T) {
	missing := GRPCErrHandler(context.Background(), e.NewBusinessErr(e.MissingCredentialsErr, nil))
	signExpired := GRPCErrHandler(context.Background(), e.NewBusinessErr(e.SignTimeErr, nil))

	if missing.Code() != signExpired.Code() {
		t.Fatalf("expected same status code; got %v vs %v", missing.Code(), signExpired.Code())
	}
	if r1, r2 := errorInfoOf(t, missing).Reason, errorInfoOf(t, signExpired).Reason; r1 == r2 {
		t.Errorf("expected different reason; both %q", r1)
	}
	// 业务码必须不同。
	if c1, c2 := errorInfoOf(t, missing).Metadata[defined.MDBizCode], errorInfoOf(t, signExpired).Metadata[defined.MDBizCode]; c1 == c2 {
		t.Errorf("expected different %s; both %q", defined.MDBizCode, c1)
	}
}

// ===== message 的来源 =====

// TestGRPCErrHandler_MessageIsCodeErrMsg: message 就是 CodeErr.Msg，
// 原始错误不会出现在任何面向客户端的字段里。
//
// 【为什么不需要脱敏】CodeErr.Msg 是编译期常量（"Operation failed"），
// 底层错误只存在于 bizErr.Err，没有通道进入 status.message 或 metadata。
func TestGRPCErrHandler_MessageIsCodeErrMsg(t *testing.T) {
	inner := errors.New("db connection refused at 10.0.0.5:5432 password=hunter2")
	st := GRPCErrHandler(context.Background(), e.NewBusinessErr(e.FailedErr, inner))

	if st.Code() != codes.Internal {
		t.Errorf("Code = %v; want %v", st.Code(), codes.Internal)
	}
	if msg := st.Message(); msg != e.FailedErr.Msg {
		t.Errorf("Message = %q; want %q", msg, e.FailedErr.Msg)
	}
}

// TestGRPCErrHandler_TestEnvShowsDetail: TEST 环境把底层错误拼进 message。
func TestGRPCErrHandler_TestEnvShowsDetail(t *testing.T) {
	ctx := appctx.Identity{Service: "svc", Env: defined.TEST}.WithContext(context.Background())
	inner := errors.New("db connection refused at 10.0.0.5:5432")
	st := GRPCErrHandler(ctx, e.NewBusinessErr(e.FailedErr, inner))

	if msg := st.Message(); !strings.Contains(msg, "db connection refused") {
		t.Errorf("TEST 环境 Message = %q; 应包含底层错误", msg)
	}
}

// ===== code 只由 Class 决定 =====

// TestCodeOf_IgnoresUnderlyingStatus 底层包着 status 也不影响 code。
//
// 【为什么要有这条】曾经 codeOf 会从 bizErr.Err 里透传 status code，
// 导致 code 来自底层 status、reason/message/biz_code 来自 CodeErr，
// 四个字段分成两个来源，线上产物自相矛盾：
//
//	code=NotFound  reason="INTERNAL"  message="Operation failed"
//
// 现在统一以 Class 为准，Err 只用于日志和告警。
func TestCodeOf_IgnoresUnderlyingStatus(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode codes.Code
		wantMsg  string
	}{
		{
			name:     "裸 status",
			err:      status.Error(codes.NotFound, "order 123 not found"),
			wantCode: codes.Internal,
			wantMsg:  e.FailedErr.Msg,
		},
		{
			name:     "fmt.Errorf 包装 status",
			err:      fmt.Errorf("查订单失败: %w", status.Error(codes.NotFound, "order 123 not found")),
			wantCode: codes.Internal,
			wantMsg:  e.FailedErr.Msg,
		},
		{
			name:     "显式声明 DataNotExist，底层是 Unavailable",
			err:      e.NewBusinessErr(e.DataNotExist, status.Error(codes.Unavailable, "db down")),
			wantCode: codes.NotFound,
			wantMsg:  e.DataNotExist.Msg,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := GRPCErrHandler(context.Background(), c.err)
			info := errorInfoOf(t, st)

			if st.Code() != c.wantCode {
				t.Errorf("code = %v; want %v", st.Code(), c.wantCode)
			}
			if st.Message() != c.wantMsg {
				t.Errorf("message = %q; want %q", st.Message(), c.wantMsg)
			}
			// reason 是业务文案，与 code 无关；code 只由 Class 决定。
			if info.Reason != c.wantMsg {
				t.Errorf("reason = %q; want %q", info.Reason, c.wantMsg)
			}
		})
	}
}

// TestCodeOf_ExplicitStatusStillPassesThrough 业务直接返回 status 时，
// ErrorUnaryInterceptor 会原样放行 —— 透传的出口还在，只是不在 codeOf 里。
func TestCodeOf_ExplicitStatusStillPassesThrough(t *testing.T) {
	downstream := status.Error(codes.NotFound, "order 123 not found")

	_, err := ErrorUnaryInterceptor()(context.Background(), nil, nil,
		func(context.Context, any) (any, error) { return nil, downstream })

	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v; want NotFound（直接返回 status 应原样透传）", status.Code(err))
	}
	if got := status.Convert(err).Message(); got != "order 123 not found" {
		t.Errorf("message = %q; want 原文透传", got)
	}
}

// ===== domain =====

// TestDomainOf_FromContext 身份从 ctx 取，优先于全局默认值。
//
// 【为什么】多实例/多租户下同一进程可能持有不同身份，读全局会串。
func TestDomainOf_FromContext(t *testing.T) {
	prev := appctx.Default()
	appctx.SetDefault(appctx.Identity{Service: "global-service"})
	t.Cleanup(func() { appctx.SetDefault(prev) })

	ctx := appctx.Identity{Service: "ctx-service", Env: "prod"}.WithContext(context.Background())
	if got := domainOf(ctx); got != "ctx-service" {
		t.Errorf("domainOf(ctx) = %q; want %q（必须优先于全局默认值）", got, "ctx-service")
	}
}

// TestDomainOf_UnknownWhenMissing 服务名缺失时返回 "unknown"，不拿库名充数。
//
// 【为什么】domain 是 (reason, domain) 唯一性的一半。填 "go-lib" 等于
// 谎称错误来自一个叫 go-lib 的服务；客户端会据此做出错误判断。
func TestDomainOf_UnknownWhenMissing(t *testing.T) {
	prev := appctx.Default()
	appctx.SetDefault(appctx.Identity{})
	t.Cleanup(func() { appctx.SetDefault(prev) })

	if got := domainOf(context.Background()); got != defined.UnknownGRPCDomain {
		t.Errorf("domainOf() = %q; want %q", got, defined.UnknownGRPCDomain)
	}
}

// ===== 字段级校验错误 =====

// TestGRPCErrHandler_FieldViolations: 字段校验错误转成 BadRequest
func TestGRPCErrHandler_FieldViolations(t *testing.T) {
	violations := []e.FieldViolation{
		{Field: "amount", Description: "amount is required"},
		{Field: "currency", Description: "currency must be CNY"},
	}
	bizErr := e.NewBusinessErr(e.ParamErr, nil).SetData(violations)

	st := GRPCErrHandler(context.Background(), bizErr)

	var bad *errdetails.BadRequest
	for _, detail := range st.Details() {
		if b, ok := detail.(*errdetails.BadRequest); ok {
			bad = b
			break
		}
	}
	if bad == nil {
		t.Fatal("expected BadRequest in details")
	}
	if len(bad.FieldViolations) != 2 {
		t.Fatalf("got %d violations; want 2", len(bad.FieldViolations))
	}
	if bad.FieldViolations[0].Field != "amount" {
		t.Errorf("Field = %q; want %q", bad.FieldViolations[0].Field, "amount")
	}
}

// ===== details 是数组 =====

// TestGRPCErrHandler_DetailsIsArray: details 是数组，可挂多种标准 detail
func TestGRPCErrHandler_DetailsIsArray(t *testing.T) {
	violations := []e.FieldViolation{{Field: "amount", Description: "required"}}
	bizErr := e.NewBusinessErr(e.ParamErr, nil).SetData(violations)

	st := GRPCErrHandler(context.Background(), bizErr)

	details := st.Details()
	if len(details) != 2 {
		t.Fatalf("got %d details; want 2 (ErrorInfo + BadRequest)", len(details))
	}
	if _, ok := details[0].(*errdetails.ErrorInfo); !ok {
		t.Errorf("details[0] = %T; want *errdetails.ErrorInfo", details[0])
	}
	if _, ok := details[1].(*errdetails.BadRequest); !ok {
		t.Errorf("details[1] = %T; want *errdetails.BadRequest", details[1])
	}
}

// ===== helper =====

// errorInfoOf 从 status 的 details 里取出 ErrorInfo。
func errorInfoOf(t *testing.T, st *status.Status) *errdetails.ErrorInfo {
	t.Helper()
	for _, detail := range st.Details() {
		if info, ok := detail.(*errdetails.ErrorInfo); ok {
			return info
		}
	}
	t.Fatal("no ErrorInfo in details")
	return nil
}
