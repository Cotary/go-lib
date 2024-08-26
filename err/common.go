package e

const (
	SystemErrCode = iota + 10001
	FailedErrCode
	NeedLoginErrCode
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
)

var (
	SystemErr            = NewCodeErr(SystemErrCode, "System abnormality", PanicLevel)
	FailedErr            = NewCodeErr(FailedErrCode, "Operation failed", ErrorLevel)
	NeedLoginErr         = NewCodeErr(NeedLoginErrCode, "Need Login", InfoLevel)
	ParamErr             = NewCodeErr(ParamErrCode, "Params Error", InfoLevel)
	DataNotExist         = NewCodeErr(DataNotExistCode, "Data Not Exist", InfoLevel)
	DataExist            = NewCodeErr(DataExistCode, "Data Exist", InfoLevel)
	AuthErr              = NewCodeErr(AuthErrCode, "Auth Error", InfoLevel)
	SignTimeErr          = NewCodeErr(SignTimeErrCode, "Sign Time Error", InfoLevel)
	SignErr              = NewCodeErr(SignErrCode, "Sign Error", InfoLevel)
	SignReplayErr        = NewCodeErr(SignReplayErrCode, "Sign Replay Error", InfoLevel)
	VerifyErr            = NewCodeErr(VerifyErrCode, "Verify Error", InfoLevel)
	PermissionErr        = NewCodeErr(PermissionErrCode, "Permission Denied", InfoLevel)
	TimeoutErr           = NewCodeErr(TimeoutErrCode, "Request Timeout", InfoLevel)
	ServiceBusy          = NewCodeErr(ServiceBusyErrCode, "The service is busy", InfoLevel)
	RequestLimitExceeded = NewCodeErr(LimitExceedCode, "Request limit exceeded", InfoLevel)
)
