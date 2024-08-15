package e

import (
	"fmt"
	"github.com/pkg/errors"
	"runtime"
	"strings"
)

func Err(err error, message ...string) error {
	str := strings.Join(message, "-")
	if codeErr, ok := err.(*CodeErr); ok {
		return NewHttpErr(codeErr, errors.New(str))
	}

	if err == nil {
		if str != "" {
			return errors.New(str)
		} else {
			return nil
		}
	}

	hasStack := GetStakeErr(err) != nil
	if hasStack {
		if len(message) > 0 {
			return errors.WithMessage(err, str)
		}
		return err
	}
	// The original error doesn't have a stack trace. Add a stack trace.
	if len(message) > 0 {
		return errors.Wrap(err, str)
	}
	return errors.WithStack(err)
}

func GetStakeErr(err error) error {
	for unwrapErr := err; unwrapErr != nil; {
		if _, ok := unwrapErr.(interface {
			StackTrace() errors.StackTrace
		}); ok {
			return unwrapErr
		}
		u, ok := unwrapErr.(interface {
			Unwrap() error
		})
		if !ok {
			break
		}
		unwrapErr = u.Unwrap()
	}
	return nil
}

func GetErrMessage(err error) string {
	stackList := make([]error, 0)
	for unwrapErr := err; unwrapErr != nil; {
		stackList = append(stackList, unwrapErr)
		u, ok := unwrapErr.(interface {
			Unwrap() error
		})
		if !ok {
			break
		}
		unwrapErr = u.Unwrap()
	}
	allLevel := len(stackList)
	str := "\n"
	for i, e := range stackList {
		str += fmt.Sprintf("[%d]:%s\n", i+1, e.Error())
		if stackErr, ok := e.(interface {
			StackTrace() errors.StackTrace
		}); ok {
			str += fmt.Sprintf("\nstack:\n")
			isFirstErr := allLevel == i+1
			for si, sf := range stackErr.StackTrace() {
				if !isFirstErr && si == 0 {
					continue
				}
				pc := uintptr(sf) - 1
				fn := runtime.FuncForPC(pc)
				file, line := fn.FileLine(pc)
				str += fmt.Sprintf("%s:%d\n", file, line)
			}
			str += fmt.Sprintf("\n")
		}
	}
	return str
}
