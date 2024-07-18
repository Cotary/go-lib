package e

import (
	"context"
	"fmt"
	"github.com/Cotary/go-lib"
	"github.com/Cotary/go-lib/common/defined"
	"github.com/Cotary/go-lib/common/utils"
	"github.com/Cotary/go-lib/log"
	"github.com/pkg/errors"
	"runtime"
	"strings"
)

func Err(err error, message ...string) error {
	str := strings.Join(message, "-")
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

type StackTracer interface {
	StackTrace() errors.StackTrace
}

func GetStakeErr(err error) error {
	for unwrapErr := err; unwrapErr != nil; {
		if _, ok := unwrapErr.(StackTracer); ok {
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
		if stackErr, ok := e.(StackTracer); ok {
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

var messageSender func(ctx context.Context, zMap utils.ZMap[string, string])

func SetMessageSender(sender func(ctx context.Context, zMap utils.ZMap[string, string])) {
	messageSender = sender
}

func SendMessage(ctx context.Context, err error) {
	errMsg := GetErrMessage(Err(err))

	env := lib.Env
	serverName := lib.ServerName
	requestID, _ := ctx.Value(defined.RequestID).(string)

	log.WithContext(ctx).
		WithField("ServerName", serverName).
		WithField("Env", env).
		WithField("Error", errMsg).
		Error(errMsg)

	zMap := utils.NewZMap[string, string]().
		Set("ServerName:", serverName).
		Set("Env:", env).
		Set("RequestID:", requestID).
		Set("Error:", errMsg)

	messageSender(ctx, *zMap)
}
