package e

// ============================================================================
// 错误级别
// ============================================================================

// Level type
//
// 数值越小越严重。传输层收口点用 `Level <= WarnLevel` 判断是否告警，
// 这是全项目唯一的告警触发条件。
type Level uint32

const (
	// PanicLevel level, highest level of severity. Logs and then calls panic with the
	// message passed to Debug, Info, ...
	PanicLevel Level = iota
	// FatalLevel level. Logs and then calls `logger.Exit(1)`. It will exit even if the
	// logging level is set to Panic.
	FatalLevel
	// ErrorLevel level. Logs. Used for errors that should definitely be noted.
	// Commonly used for hooks to send errors to an error tracking service.
	ErrorLevel
	// WarnLevel level. Non-critical entries that deserve eyes.
	WarnLevel
	// InfoLevel level. General operational entries about what's going on inside the
	// application.
	InfoLevel
	// DebugLevel level. Usually only enabled when debugging. Very verbose logging.
	DebugLevel
	// TraceLevel level. Designates finer-grained informational events than the Debug.
	TraceLevel
)

// ============================================================================
// 错误类别
// ============================================================================

// Class 错误类别：与传输协议无关的「错误性质」。
//
// 它的取值刻意与 google.rpc.Code 一致（0~16），所以 gRPC 传输层可以直接
// 映射成 status.Code，而 err 包本身不依赖 grpc。
//
// 三个维度各管一件事：
//
//	Class —— 类别。协议层的错误表达（gRPC status code）。
//	Code  —— 业务码。服务内唯一标识一个错误，errors.Is 按它判定。
//	Msg   —— 面向用户的文案。
//
// 【为什么不需要单独的 Reason 字段】
// gRPC 的 google.rpc.ErrorInfo.reason 字段要的是「机器可读的常量」，
// 而类别本身就是那个常量（NOT_FOUND / UNAUTHENTICATED ...），
// 没必要再造一个与 Msg 重复的字段。
// 同类别下的细分由业务码区分，它进 ErrorInfo.metadata.biz_code。
type Class uint32

const (
	ClassOK                 Class = 0
	ClassCanceled           Class = 1
	ClassUnknown            Class = 2
	ClassInvalidArgument    Class = 3
	ClassDeadlineExceeded   Class = 4
	ClassNotFound           Class = 5
	ClassAlreadyExists      Class = 6
	ClassPermissionDenied   Class = 7
	ClassResourceExhausted  Class = 8
	ClassFailedPrecondition Class = 9
	ClassAborted            Class = 10
	ClassOutOfRange         Class = 11
	ClassUnimplemented      Class = 12
	ClassInternal           Class = 13
	ClassUnavailable        Class = 14
	ClassDataLoss           Class = 15
	ClassUnauthenticated    Class = 16
)

// String 返回类别的 Go 风格名称。
func (c Class) String() string {
	switch c {
	case ClassOK:
		return "OK"
	case ClassCanceled:
		return "Canceled"
	case ClassUnknown:
		return "Unknown"
	case ClassInvalidArgument:
		return "InvalidArgument"
	case ClassDeadlineExceeded:
		return "DeadlineExceeded"
	case ClassNotFound:
		return "NotFound"
	case ClassAlreadyExists:
		return "AlreadyExists"
	case ClassPermissionDenied:
		return "PermissionDenied"
	case ClassResourceExhausted:
		return "ResourceExhausted"
	case ClassFailedPrecondition:
		return "FailedPrecondition"
	case ClassAborted:
		return "Aborted"
	case ClassOutOfRange:
		return "OutOfRange"
	case ClassUnimplemented:
		return "Unimplemented"
	case ClassInternal:
		return "Internal"
	case ClassUnavailable:
		return "Unavailable"
	case ClassDataLoss:
		return "DataLoss"
	case ClassUnauthenticated:
		return "Unauthenticated"
	default:
		return "Unset"
	}
}

// Reason 返回类别的 UPPER_SNAKE_CASE 名称。
//
// 它和 google.rpc.Code 的枚举名完全一致，正是 gRPC 的
// google.rpc.ErrorInfo.reason 字段要求的格式（[A-Z][A-Z0-9_]+[A-Z0-9]）。
//
// 【注意】google.rpc 里叫 CANCELLED，grpc-go 里叫 Canceled。
// 这里取 google.rpc 的拼写，因为 reason 走的是 google.rpc 的规范。
func (c Class) Reason() string {
	switch c {
	case ClassOK:
		return "OK"
	case ClassCanceled:
		return "CANCELLED"
	case ClassUnknown:
		return "UNKNOWN"
	case ClassInvalidArgument:
		return "INVALID_ARGUMENT"
	case ClassDeadlineExceeded:
		return "DEADLINE_EXCEEDED"
	case ClassNotFound:
		return "NOT_FOUND"
	case ClassAlreadyExists:
		return "ALREADY_EXISTS"
	case ClassPermissionDenied:
		return "PERMISSION_DENIED"
	case ClassResourceExhausted:
		return "RESOURCE_EXHAUSTED"
	case ClassFailedPrecondition:
		return "FAILED_PRECONDITION"
	case ClassAborted:
		return "ABORTED"
	case ClassOutOfRange:
		return "OUT_OF_RANGE"
	case ClassUnimplemented:
		return "UNIMPLEMENTED"
	case ClassInternal:
		return "INTERNAL"
	case ClassUnavailable:
		return "UNAVAILABLE"
	case ClassDataLoss:
		return "DATA_LOSS"
	case ClassUnauthenticated:
		return "UNAUTHENTICATED"
	default:
		return "UNKNOWN"
	}
}

// ============================================================================
// 业务错误
// ============================================================================

// CodeErr 是协议无关的业务错误声明。
//
// 业务层只负责声明「这是什么错、什么类别、什么级别」，
// 不写日志、不发告警、不关心 HTTP 还是 gRPC——那是传输层收口点的事。
type CodeErr struct {
	// Code 业务码。一个服务内唯一标识一个错误，errors.Is 按它判定。
	Code int `json:"code"`
	// Msg 面向用户的文案。
	Msg string `json:"message"`
	// Level 告警级别。<= WarnLevel 的错误会在传输层收口点触发告警。
	Level Level `json:"-"`
	// Class 错误类别。传输层据此决定协议层的错误表达。
	Class Class `json:"-"`
}

// NewCodeErr 构造业务错误。
//
//	code  业务码，服务内唯一
//	class 错误类别，对应 google.rpc.Code 的语义
//	msg   面向用户的文案
//	level 告警级别
func NewCodeErr(code int, class Class, msg string, level Level) *CodeErr {
	return &CodeErr{
		Code:  code,
		Msg:   msg,
		Level: level,
		Class: class,
	}
}

// Error 返回面向用户的文案。
func (e *CodeErr) Error() string {
	return e.Msg
}

// RewriteMsg 换掉文案，保留 code / class / level。
func (e *CodeErr) RewriteMsg(msg string) *CodeErr {
	return NewCodeErr(e.Code, e.Class, msg, e.Level)
}

// WithClass 换掉类别，返回新实例。
func (e *CodeErr) WithClass(class Class) *CodeErr {
	return NewCodeErr(e.Code, class, e.Msg, e.Level)
}

// WithLevel 换掉告警级别，返回新实例。
func (e *CodeErr) WithLevel(level Level) *CodeErr {
	return NewCodeErr(e.Code, e.Class, e.Msg, level)
}

// Is 让 errors.Is 按业务码判定，而不是按指针比较。
//
// 【人写】业务码在服务内唯一标识一个错误，所以只比 Code。
// 类别不参与判定：同一个类别下可以有多个不同错误
// （比如「订单不存在」和「用户不存在」都是 NotFound）。
func (e *CodeErr) Is(target error) bool {
	t, ok := target.(*CodeErr)
	if !ok {
		return false
	}
	return e.Code == t.Code
}
