package e

// ============================================================================
// 业务错误码
// ============================================================================

// 业务码分三段：
//
//	10001+   go-lib 基础设施类错误（系统异常、操作失败、取消、自定义）
//	10101+   go-lib 业务语义类错误（凭证、参数、数据、限流等）
//	20001+   业务自定义错误（见 example/common/defined/errors.go）
//
// 都避开 0~16（gRPC 状态码）和 0（HTTP 成功码），客户端还原时不会撞车。
//
// 业务码只在服务内有意义，跨网络传输时会被放进
// google.rpc.ErrorInfo.metadata.biz_code，客户端据此还原。

const (
	SystemErrCode = iota + 10001
	FailedErrCode
	CanceledErrCode
	CustomErrCode
)
const (
	MissingCredentialsErrCode = iota + 10101
	ParamErrCode
	DataNotExistCode
	DataExistCode
	AuthErrCode
	SignTimeErrCode
	SignErrCode
	SignReplayErrCode
	VerifyErrCode
	PermissionErrCode
	TimeoutErrCode
	ServiceBusyErrCode
	LimitExceedCode
	ConfigErrCode
)

// ============================================================================
// 内置业务错误
// ============================================================================

// Level 决定告警级别，传输层收口点按 `Level <= WarnLevel` 判断是否告警：
//
//	InfoLevel  业务正常分支（数据不存在、参数不合法）→ 只记日志，不告警
//	WarnLevel  边缘情况 → 告警
//	ErrorLevel 需要人关注 → 告警
//	PanicLevel 严重故障 → 立即告警
//
// Class 决定协议层的错误表达（gRPC 的 status code）。
var (
	// MissingCredentialsErr 表示请求没带凭证（没登录）。
	//
	// 和 AuthErr / SignErr 的区别：那些是「带了凭证但无效」，
	// 这个是「压根没带」。客户端据此决定是跳登录页还是提示重新认证。
	CustomErr = NewCodeErr(CustomErrCode, ClassUnknown, "Custom Error", InfoLevel)
	Canceled  = NewCodeErr(CanceledErrCode, ClassCanceled, "Request canceled", InfoLevel)

	SystemErr             = NewCodeErr(SystemErrCode, ClassInternal, "System abnormality", PanicLevel)
	FailedErr             = NewCodeErr(FailedErrCode, ClassInternal, "Operation failed", ErrorLevel)
	ParamErr              = NewCodeErr(ParamErrCode, ClassInvalidArgument, "Params Error", InfoLevel)
	MissingCredentialsErr = NewCodeErr(MissingCredentialsErrCode, ClassUnauthenticated, "Login credentials missing", InfoLevel)
	DataNotExist          = NewCodeErr(DataNotExistCode, ClassNotFound, "Data Not Exist", InfoLevel)
	DataExist             = NewCodeErr(DataExistCode, ClassAlreadyExists, "Data Exist", InfoLevel)
	AuthErr               = NewCodeErr(AuthErrCode, ClassUnauthenticated, "Auth Error", InfoLevel)
	SignTimeErr           = NewCodeErr(SignTimeErrCode, ClassUnauthenticated, "Sign Time Error", InfoLevel)
	SignErr               = NewCodeErr(SignErrCode, ClassUnauthenticated, "Sign Error", InfoLevel)
	SignReplayErr         = NewCodeErr(SignReplayErrCode, ClassUnauthenticated, "Sign Replay Error", InfoLevel)
	VerifyErr             = NewCodeErr(VerifyErrCode, ClassPermissionDenied, "Verify Error", InfoLevel)
	PermissionErr         = NewCodeErr(PermissionErrCode, ClassPermissionDenied, "Permission Denied", InfoLevel)
	TimeoutErr            = NewCodeErr(TimeoutErrCode, ClassDeadlineExceeded, "Request Timeout", InfoLevel)
	ServiceBusy           = NewCodeErr(ServiceBusyErrCode, ClassResourceExhausted, "The service is busy", InfoLevel)
	RequestLimitExceeded  = NewCodeErr(LimitExceedCode, ClassResourceExhausted, "Request limit exceeded", InfoLevel)
	ConfigErr             = NewCodeErr(ConfigErrCode, ClassFailedPrecondition, "Config Error", InfoLevel)
)

// ============================================================================
// 取消
// ============================================================================

// Canceled 表示请求被客户端取消。
//
// 它由 AsBusinessErr 在遇到 context.Canceled 时自动归一化产生，
// 业务层不需要（也不应该）主动使用。
// 业务码不能是 0：0 是成功码（HTTP 侧 response.Success 用的就是 0），
// 用 0 会让「请求被取消」在客户端被当成成功处理。
