package e

import "context"

// Level type
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

type CodeErr struct {
	Code  int    `json:"code"`    //内置的http错误
	Msg   string `json:"message"` //内置的http错误
	Level Level  `json:"-"`       //内置的http错误等级
}

func (err *CodeErr) Error() string {
	return err.Msg
}

func NewCodeErr(code int, msg string, level Level) *CodeErr {
	return &CodeErr{
		Code:  code,
		Msg:   msg,
		Level: level,
	}
}

// HttpErr http错误,把data放在这个里面，避免污染CodeErr指针
type HttpErr struct {
	*CodeErr             //内置的http错误
	Err      error       //真实错误
	Data     interface{} `json:"data"`
}

func NewHttpErr(codeErr *CodeErr, err error) HttpErr {
	return HttpErr{
		CodeErr: codeErr,
		Err:     Err(err),
	}

}

func (t HttpErr) Error() string {
	return t.CodeErr.Error()
}

func (t HttpErr) Unwrap() error {
	return t.Err
}

func (t HttpErr) SetData(data interface{}) error {
	t.Data = data
	return t
}

func (t HttpErr) SendErrorMsg(ctx context.Context) {
	if t.Level <= WarnLevel {
		SendMessage(ctx, t)
	}

}
