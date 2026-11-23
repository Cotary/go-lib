package e

import (
	"context"
	"errors"
)

// BusinessErr 是协议无关的业务错误：一个错误声明 + 真实错误 + 附加数据。
//
// 它是传输层收口点处理的对象：HTTP 把它渲染成 JSON body，
// gRPC 把它渲染成 status + details。
type BusinessErr struct {
	*CodeErr
	// Err 真实错误。内部用（日志、告警、堆栈），不发给客户端。
	Err error `json:"-"`
	// Data 附加数据。发给客户端，用于补充错误上下文（如登录入口）。
	Data interface{} `json:"data"`
}

// NewBusinessErr 构造业务错误。errs 可选，传底层错误。
func NewBusinessErr(codeErr *CodeErr, errs ...error) *BusinessErr {
	var err error
	if len(errs) > 0 {
		err = errs[0]
	}
	return &BusinessErr{
		CodeErr: codeErr,
		Err:     Err(err, codeErr.Msg),
	}
}

// Error 返回面向用户的文案。
func (t *BusinessErr) Error() string {
	if t.CodeErr == nil {
		return ""
	}
	return t.CodeErr.Error()
}

// Unwrap 交出真实错误，让 errors.Is / errors.As 能继续往里穿透。
func (t *BusinessErr) Unwrap() error {
	return t.Err
}

// SetData 附加数据，返回自身以便链式调用。
func (t *BusinessErr) SetData(data interface{}) *BusinessErr {
	t.Data = data
	return t
}

// AsBusinessErr 把任意 error 归一化成 BusinessErr。
//
// 它是全项目唯一的错误提取入口，所有传输层的收口点都走它：
//
//	裸 error     → FailedErr（ClassInternal）
//	*CodeErr     → 包装成 BusinessErr
//	*BusinessErr → 原样返回
//	context 错误  → Canceled / TimeoutErr
func AsBusinessErr(err error) *BusinessErr {
	if err == nil {
		return nil
	}

	var asBusiness *BusinessErr
	if errors.As(err, &asBusiness) {
		return asBusiness
	}

	var asCode *CodeErr
	if errors.As(err, &asCode) {
		return NewBusinessErr(asCode, nil)
	}

	// context 错误归一化：让 HTTP 和 gRPC 对「超时」「取消」的判定一致。
	if errors.Is(err, context.Canceled) {
		return NewBusinessErr(Canceled, err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewBusinessErr(TimeoutErr, err)
	}

	return NewBusinessErr(FailedErr, err)
}

// ============================================================================
// 字段级校验错误
// ============================================================================

// FieldViolation 是字段级校验错误。
//
// gRPC 传输层会把它转成 google.rpc.BadRequest，
// HTTP 传输层直接放进 body 的 data 字段。
type FieldViolation struct {
	Field       string `json:"field"`
	Description string `json:"description"`
}
