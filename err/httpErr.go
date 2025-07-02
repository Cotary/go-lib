package e

import (
	"runtime"

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

type stack []uintptr

func callers() *stack {
	const depth = 32
	var pcs [depth]uintptr
	n := runtime.Callers(3, pcs[:])
	var st stack = pcs[0:n]
	return &st
}

func (s *stack) StackTrace() errors.StackTrace {
	if s == nil {
		return nil
	}
	f := make([]errors.Frame, len(*s))
	for i := 0; i < len(f); i++ {
		f[i] = errors.Frame((*s)[i])
	}
	return f
}

type CodeErr struct {
	Code  int    `json:"code"`    //内置的http错误
	Msg   string `json:"message"` //内置的http错误
	Level Level  `json:"-"`       //内置的http错误等级
}

func (e *CodeErr) Error() string {
	return e.Msg
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

func HErr(codeErr *CodeErr, err ...error) *HttpErr {
	if len(err) > 0 {
		return NewHttpErr(codeErr, err[0])
	}
	return NewHttpErr(codeErr, nil)
}

func NewHttpErr(codeErr *CodeErr, err error) *HttpErr {
	if err == nil {
		return &HttpErr{
			CodeErr: codeErr,
			Err:     errors.New(""),
		}
	}
	return &HttpErr{
		CodeErr: codeErr,
		Err:     Err(err),
	}

}

func (t *HttpErr) Error() string {
	return t.CodeErr.Error()
}

func (t *HttpErr) Unwrap() error {
	return t.Err
}

func (t *HttpErr) SetData(data interface{}) error {
	t.Data = data
	return t
}
