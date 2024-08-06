package e

var (
	SystemErr    = NewCodeErr(10001, "System abnormality", PanicLevel)
	FailedErr    = NewCodeErr(10002, "Operation failed", ErrorLevel)
	ParamErr     = NewCodeErr(10003, "Params Error", InfoLevel)
	DataNotExist = NewCodeErr(10004, "Data Not Exist", InfoLevel)
	DataExist    = NewCodeErr(10005, "Data Exist", InfoLevel)
	AuthErr      = NewCodeErr(10006, "Auth Error", InfoLevel)
	SignTimeErr  = NewCodeErr(10007, "Sign Time Error", InfoLevel)
	SignErr      = NewCodeErr(10008, "Sign Error", InfoLevel)
	VerifyErr    = NewCodeErr(10009, "Verify Error", InfoLevel)

	NeedLoginErr = NewCodeErr(20000, "Need Login", InfoLevel)
)
