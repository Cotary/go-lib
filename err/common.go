package e

var (
	SystemErr    = NewCodeErr(10001, "System abnormality", PanicLevel)
	FailedErr    = NewCodeErr(10002, "Operation failed", ErrorLevel)
	ParamErr     = NewCodeErr(10003, "Params Error", InfoLevel)
	DataNotExist = NewCodeErr(10004, "Data Not Exist", InfoLevel)
	AuthErr      = NewCodeErr(10005, "Auth Error", InfoLevel)
	NeedLoginErr = NewCodeErr(10006, "Need Login", InfoLevel)
	SignTimeErr  = NewCodeErr(10007, "Sign Time Error", InfoLevel)
	SignErr      = NewCodeErr(10008, "Sign Error", InfoLevel)
	VerifyErr    = NewCodeErr(10009, "Verify Error", InfoLevel)
)
