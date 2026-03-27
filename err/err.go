package e

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/pkg/errors"
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

	hasStack := GetStackErr(err) != nil
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

func GetStackErr(err error) error {
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

func GetErrMessage(err error, stopAtFirstStack bool) string {
	if err == nil {
		return ""
	}

	// 1. 收集从最外层到最里层的错误链
	var stackList []error
	for e := err; e != nil; {
		stackList = append(stackList, e)
		unwrapper, ok := e.(interface{ Unwrap() error })
		if !ok {
			break
		}
		e = unwrapper.Unwrap()
	}

	errMsgs := make([]string, len(stackList))
	for i, e := range stackList {
		errMsgs[i] = e.Error()
	}

	var sb strings.Builder
	sb.WriteString("\n")

	for i, e := range stackList {
		// 判断当前错误是否带有 StackTrace
		type stackTracer interface{ StackTrace() errors.StackTrace }
		sErr, hasStack := e.(stackTracer)

		// 如果和上一个错误消息一致且无 stack，则跳过打印该行
		if i > 0 && errMsgs[i] == errMsgs[i-1] && !hasStack {
			continue
		}

		// 2. 打印错误消息
		sb.WriteString(fmt.Sprintf("[%d]: %s\n", i+1, errMsgs[i]))

		// 3. 处理堆栈打印
		if hasStack {
			sb.WriteString("\n")
			isOuter := i == len(stackList)-1
			st := sErr.StackTrace()

			for si, frame := range st {
				// 非最外层跳过首帧（通常是 pkg/errors 的包装点）
				if !isOuter && si == 0 {
					continue
				}
				pc := uintptr(frame) - 1
				fn := runtime.FuncForPC(pc)
				if fn != nil {
					file, line := fn.FileLine(pc)
					sb.WriteString(fmt.Sprintf("%s:%d\n", file, line))
				}
			}
			sb.WriteString("\n")

			// --- 新增逻辑：如果开启了截断且找到了堆栈，直接结束循环 ---
			if stopAtFirstStack {
				break
			}
		}
	}

	return sb.String()
}
