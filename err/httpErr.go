package e

import (
	"github.com/pkg/errors"
)

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
	Code  int    `json:"code"`
	Msg   string `json:"message"`
	Level Level  `json:"-"`
}

func (e *CodeErr) Error() string {
	return e.Msg
}

func (e *CodeErr) RewriteMsg(msg string) *CodeErr {
	return NewCodeErr(e.Code, msg, e.Level)
}

func NewCodeErr(code int, msg string, level Level) *CodeErr {
	return &CodeErr{
		Code:  code,
		Msg:   msg,
		Level: level,
	}
}

func AsCodeErr(err error) *CodeErr {
	if err == nil {
		return nil
	}
	var asCodeErr *CodeErr
	var asHttpErr *HttpErr

	if errors.As(err, &asCodeErr) {
		return asCodeErr
	}

	if errors.As(err, &asHttpErr) {
		return asHttpErr.CodeErr
	}

	return FailedErr
}

// HttpErr http错误,把data放在这个里面，避免污染CodeErr指针
type HttpErr struct {
	*CodeErr             //内置的http错误
	Err      error       //真实错误
	Data     interface{} `json:"data"`
}

func NewHttpErr(codeErr *CodeErr, errs ...error) *HttpErr {
	var err error
	if len(errs) > 0 && errs[0] != nil {
		err = errs[0]
	}
	return &HttpErr{
		CodeErr: codeErr,
		Err:     Err(err, codeErr.Msg),
	}
}

func (t *HttpErr) Error() string {
	return t.CodeErr.Error()
}

func (t *HttpErr) Unwrap() error {
	return t.Err
}

func (t *HttpErr) SetData(data interface{}) *HttpErr {
	t.Data = data
	return t
}

func AsHttpErr(err error) *HttpErr {
	if err == nil {
		return nil
	}
	var (
		asHttp *HttpErr
		asCode *CodeErr
	)
	if errors.As(err, &asHttp) {
		return asHttp
	}
	if errors.As(err, &asCode) {
		return NewHttpErr(asCode, nil)
	}
	return NewHttpErr(FailedErr, err)
}
