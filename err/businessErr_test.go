package e

import (
	"context"
	"errors"
	"testing"

	pkgerrors "github.com/pkg/errors"
)

// ===== AsBusinessErr 归一化 =====

// TestAsBusinessErr_Nil: nil 输入返回 nil
func TestAsBusinessErr_Nil(t *testing.T) {
	if got := AsBusinessErr(nil); got != nil {
		t.Fatalf("AsBusinessErr(nil) = %v; want nil", got)
	}
}

// TestAsBusinessErr_BareError: 裸 error 兜底成 FailedErr（ClassInternal）
func TestAsBusinessErr_BareError(t *testing.T) {
	got := AsBusinessErr(errors.New("boom"))
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Code != FailedErr.Code {
		t.Errorf("Code = %d; want %d", got.Code, FailedErr.Code)
	}
	if got.Class != ClassInternal {
		t.Errorf("Class = %v; want %v", got.Class, ClassInternal)
	}
	if got.Err == nil {
		t.Error("expected underlying error to be captured")
	}
}

// TestAsBusinessErr_CodeErr: *CodeErr 被包装成 BusinessErr，保留全部字段
func TestAsBusinessErr_CodeErr(t *testing.T) {
	got := AsBusinessErr(ParamErr)
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Code != ParamErr.Code {
		t.Errorf("Code = %d; want %d", got.Code, ParamErr.Code)
	}
	if got.Class != ParamErr.Class {
		t.Errorf("Class = %v; want %v", got.Class, ParamErr.Class)
	}
	if got.Level != ParamErr.Level {
		t.Errorf("Level = %v; want %v", got.Level, ParamErr.Level)
	}
	if got.Msg != ParamErr.Msg {
		t.Errorf("Msg = %q; want %q", got.Msg, ParamErr.Msg)
	}
}

// TestAsBusinessErr_AlreadyBusiness: 已是 BusinessErr 时原样返回
func TestAsBusinessErr_AlreadyBusiness(t *testing.T) {
	orig := NewBusinessErr(ParamErr, errors.New("inner"))
	got := AsBusinessErr(orig)
	if got != orig {
		t.Error("expected same instance")
	}
}

// TestAsBusinessErr_Wrapped: 被包装后仍能提取
func TestAsBusinessErr_Wrapped(t *testing.T) {
	wrapped := pkgerrors.WithMessage(NewBusinessErr(DataNotExist, nil), "ctx")
	got := AsBusinessErr(wrapped)
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Code != DataNotExist.Code {
		t.Errorf("Code = %d; want %d", got.Code, DataNotExist.Code)
	}
}

// TestAsBusinessErr_ContextCanceled: context.Canceled 归一化成 Canceled
func TestAsBusinessErr_ContextCanceled(t *testing.T) {
	got := AsBusinessErr(context.Canceled)
	if got.Class != ClassCanceled {
		t.Errorf("Class = %v; want %v", got.Class, ClassCanceled)
	}
	if got.Code != Canceled.Code {
		t.Errorf("Code = %d; want %d", got.Code, Canceled.Code)
	}
	// 业务码不能是 0：HTTP 侧 0 是成功码，混用会让「请求被取消」被当成成功。
	if got.Code == 0 {
		t.Error("Canceled 的业务码不能是 0")
	}
}

// TestAsBusinessErr_ContextDeadline: context.DeadlineExceeded 归一化成 TimeoutErr
func TestAsBusinessErr_ContextDeadline(t *testing.T) {
	got := AsBusinessErr(context.DeadlineExceeded)
	if got.Class != ClassDeadlineExceeded {
		t.Errorf("Class = %v; want %v", got.Class, ClassDeadlineExceeded)
	}
	if got.Code != TimeoutErr.Code {
		t.Errorf("Code = %d; want %d", got.Code, TimeoutErr.Code)
	}
}

// ===== CodeErr.Is =====

// TestCodeErr_Is_SameCode: 业务码相同即匹配
func TestCodeErr_Is_SameCode(t *testing.T) {
	if !errors.Is(ParamErr, ParamErr) {
		t.Error("expected match")
	}
	// 独立构造但码相同 → 仍然匹配
	other := NewCodeErr(ParamErr.Code, ClassInternal, "other", ErrorLevel)
	if !errors.Is(other, ParamErr) {
		t.Error("expected match by code")
	}
}

// TestCodeErr_Is_DiffCode: 业务码不同不匹配
func TestCodeErr_Is_DiffCode(t *testing.T) {
	if errors.Is(ParamErr, DataNotExist) {
		t.Error("expected no match")
	}
}

// TestCodeErr_Is_IgnoresClass: 类别不参与判定
//
// 【人写】同一类别下可以有多个不同错误
// （比如「订单不存在」和「用户不存在」都是 NotFound），
// 所以判定只能靠业务码。
func TestCodeErr_Is_IgnoresClass(t *testing.T) {
	differentClass := NewCodeErr(ParamErr.Code, ClassInternal, "Params Error", InfoLevel)
	if !errors.Is(differentClass, ParamErr) {
		t.Error("expected match: class must not affect Is")
	}
}

// TestCodeErr_Is_NonCodeErr: 目标不是 CodeErr 时不匹配
func TestCodeErr_Is_NonCodeErr(t *testing.T) {
	if errors.Is(ParamErr, errors.New("x")) {
		t.Error("expected no match")
	}
}

// TestCodeErr_Is_InBusinessErr: 包在 BusinessErr 里也能匹配
func TestCodeErr_Is_InBusinessErr(t *testing.T) {
	err := NewBusinessErr(DataNotExist, nil)
	if !errors.Is(err, DataNotExist) {
		t.Error("expected match through BusinessErr")
	}
}

// ===== Class.Reason =====

// TestClassReason_MatchesGoogleRPC: 类别的 reason 常量必须符合 google.rpc 规范
//
// 【规范】google.rpc.ErrorInfo.reason 要求 UPPER_SNAKE_CASE，
// 且枚举名与 google.rpc.Code 完全一致。
func TestClassReason_MatchesGoogleRPC(t *testing.T) {
	cases := []struct {
		class Class
		want  string
	}{
		{ClassOK, "OK"},
		{ClassCanceled, "CANCELLED"},
		{ClassUnknown, "UNKNOWN"},
		{ClassInvalidArgument, "INVALID_ARGUMENT"},
		{ClassDeadlineExceeded, "DEADLINE_EXCEEDED"},
		{ClassNotFound, "NOT_FOUND"},
		{ClassAlreadyExists, "ALREADY_EXISTS"},
		{ClassPermissionDenied, "PERMISSION_DENIED"},
		{ClassResourceExhausted, "RESOURCE_EXHAUSTED"},
		{ClassFailedPrecondition, "FAILED_PRECONDITION"},
		{ClassAborted, "ABORTED"},
		{ClassOutOfRange, "OUT_OF_RANGE"},
		{ClassUnimplemented, "UNIMPLEMENTED"},
		{ClassInternal, "INTERNAL"},
		{ClassUnavailable, "UNAVAILABLE"},
		{ClassDataLoss, "DATA_LOSS"},
		{ClassUnauthenticated, "UNAUTHENTICATED"},
	}
	for _, c := range cases {
		if got := c.class.Reason(); got != c.want {
			t.Errorf("Class(%d).Reason() = %q; want %q", c.class, got, c.want)
		}
	}
}

// TestClassReason_Format: reason 必须符合 UPPER_SNAKE_CASE 规范
func TestClassReason_Format(t *testing.T) {
	for _, c := range []Class{
		ClassOK, ClassCanceled, ClassUnknown, ClassInvalidArgument,
		ClassDeadlineExceeded, ClassNotFound, ClassAlreadyExists,
		ClassPermissionDenied, ClassResourceExhausted, ClassFailedPrecondition,
		ClassAborted, ClassOutOfRange, ClassUnimplemented, ClassInternal,
		ClassUnavailable, ClassDataLoss, ClassUnauthenticated,
	} {
		r := c.Reason()
		if r == "" {
			t.Errorf("Class(%d) has empty reason", c)
			continue
		}
		if len(r) > 63 {
			t.Errorf("reason %q too long", r)
		}
		if r[0] < 'A' || r[0] > 'Z' {
			t.Errorf("reason %q must start with uppercase letter", r)
		}
		for _, ch := range r {
			if !(ch >= 'A' && ch <= 'Z') && ch != '_' {
				t.Errorf("reason %q contains invalid char %q", r, ch)
			}
		}
	}
}

// ===== 内置错误的基本不变量 =====

// TestBuiltinErrors_ClassValid: 所有内置错误的 Class 必须是已知值
func TestBuiltinErrors_ClassValid(t *testing.T) {
	all := []*CodeErr{
		MissingCredentialsErr, CustomErr, SystemErr, FailedErr, ParamErr, DataNotExist,
		DataExist, AuthErr, SignTimeErr, SignErr, SignReplayErr, VerifyErr,
		PermissionErr, TimeoutErr, ServiceBusy, RequestLimitExceeded, ConfigErr,
		Canceled,
	}
	for _, ce := range all {
		if ce.Class > ClassUnauthenticated {
			t.Errorf("%s: unknown class %v", ce.Msg, ce.Class)
		}
	}
}

// TestBuiltinErrors_CodeUnique: 业务码必须唯一
//
// 【人写】业务码是 errors.Is 的判定依据，重复会让两个不同错误无法区分。
func TestBuiltinErrors_CodeUnique(t *testing.T) {
	all := []*CodeErr{
		MissingCredentialsErr, CustomErr, SystemErr, FailedErr, ParamErr, DataNotExist,
		DataExist, AuthErr, SignTimeErr, SignErr, SignReplayErr, VerifyErr,
		PermissionErr, TimeoutErr, ServiceBusy, RequestLimitExceeded, ConfigErr,
	}
	seen := map[int]string{}
	for _, ce := range all {
		if prev, ok := seen[ce.Code]; ok {
			t.Errorf("duplicate code %d: %s and %s", ce.Code, prev, ce.Msg)
		}
		seen[ce.Code] = ce.Msg
	}
}

// TestRewriteMsg_KeepsFields: RewriteMsg 保留 code/class/level
func TestRewriteMsg_KeepsFields(t *testing.T) {
	got := ParamErr.RewriteMsg("new msg")
	if got.Msg != "new msg" {
		t.Errorf("Msg = %q; want %q", got.Msg, "new msg")
	}
	if got.Code != ParamErr.Code || got.Class != ParamErr.Class || got.Level != ParamErr.Level {
		t.Error("RewriteMsg must preserve code/class/level")
	}
	// 原实例不能被改动
	if ParamErr.Msg != "Params Error" {
		t.Error("original must not be mutated")
	}
}
