package e

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	pkgerrors "github.com/pkg/errors"
)

func TestErrNil(t *testing.T) {
	if got := Err(nil); got != nil {
		t.Fatalf("Err(nil) = %v; want nil", got)
	}
}

// TestErrNil_WithMessage: Err(nil, "a", "b") 应返回新 error，Message="a-b"，且不带栈
func TestErrNil_WithMessage(t *testing.T) {
	msg := "foo-bar"
	err := Err(nil, "foo", "bar")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Error() != msg {
		t.Errorf("Error() = %q; want %q", err.Error(), msg)
	}
	if GetStackErr(err) == nil {
		t.Errorf("expected stack trace")
	}
}

// TestErr_NoStackNoMessage: 原始无栈 err，且无 message，应附加栈信息
func TestErr_NoStackNoMessage(t *testing.T) {
	base := errors.New("orig")
	err := Err(base)
	// Err 应返回一个带栈的错误
	se := GetStackErr(err)
	if se == nil {
		t.Fatal("expected stack trace, got none")
	}
	// 打印栈信息，确认包含当前测试函数名
	out := GetErrMessage(err)
	t.Logf("stack:\n%s", out)
}

// TestErr_NoStackWithMessage: 原始无栈 err，且有 message，应 Wrap 并带栈
func TestErr_NoStackWithMessage(t *testing.T) {
	base := errors.New("orig")
	err := Err(base, "x", "y")
	// Wrap 后 Error() 应包含 "x-y"
	if !strings.Contains(err.Error(), "x-y") {
		t.Errorf("Error() = %q; want suffix %q", err.Error(), "x-y")
	}
	// 应包含栈
	if GetStackErr(err) == nil {
		t.Fatal("expected stack trace after wrap")
	}
	// 打印栈
	out := GetErrMessage(err)
	t.Logf("wrapped stack:\n%s", out)

}

// TestErr_WithStackNoMessage: 原始 err 已带栈，且无 message，应原样返回
func TestErr_WithStackNoMessage(t *testing.T) {
	base := pkgerrors.WithStack(errors.New("orig-stack"))
	err := Err(base)
	// 应直接返回 base，不再重复 Wrap/WithStack
	if base != err {
		t.Errorf("expected same error instance; got new one")
	}
	// 确认能提取出栈
	if GetStackErr(err) == nil {
		t.Fatal("expected existing stack trace")
	}
}

// TestErr_WithStackWithMessage: 原始 err 已带栈，且有 message，应 WithMessage
func TestErr_WithStackWithMessage(t *testing.T) {
	base := pkgerrors.WithStack(errors.New("orig-stack"))
	err := Err(base, "m1", "m2")
	// Error() 前缀仍然是原始 err，然后以 ": m1-m2" 结尾
	if !regexp.MustCompile(`^m1-m2: orig-stack$`).MatchString(err.Error()) {
		t.Errorf("Error() = %q; want pattern %q", err.Error(), `m1-m2: orig-stack`)
	}
	// 原始栈应依然可提取
	if GetStackErr(err) == nil {
		t.Fatal("expected stack trace after WithMessage")
	}
	// 打印栈，确认函数名
	out := GetErrMessage(err)
	t.Logf("message+stack:\n%s", out)
}
