package e

import (
	"fmt"
	"os"
	"path/filepath"
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

func GetErrMessage(err error) string {
	if err == nil {
		return ""
	}

	// 收集从最外层到最里层的错误链
	var stackList []error
	for e := err; e != nil; {
		stackList = append(stackList, e)
		unwrapper, ok := e.(interface{ Unwrap() error })
		if !ok {
			break
		}
		e = unwrapper.Unwrap()
	}

	var sb strings.Builder
	sb.WriteString("\n")

	for i, e := range stackList {
		// 判断当前错误是否带有 StackTrace
		_, hasStack := e.(interface{ StackTrace() errors.StackTrace })

		// 如果和上一个错误消息一致且无 stack，则跳过打印该行
		isSameAsPrev := i > 0 && e.Error() == stackList[i-1].Error()
		if isSameAsPrev && !hasStack {
			continue
		}

		// 打印错误消息
		sb.WriteString(fmt.Sprintf("[%d]: %s\n", i+1, e.Error()))

		// 打印 StackTrace：最外层全部，中间层跳过首帧
		if stackErr, ok := e.(interface{ StackTrace() errors.StackTrace }); ok {
			isOuter := i == len(stackList)-1
			for si, frame := range stackErr.StackTrace() {
				if !isOuter && si == 0 {
					continue
				}
				pc := uintptr(frame) - 1
				fn := runtime.FuncForPC(pc)
				file, line := fn.FileLine(pc)
				sb.WriteString(fmt.Sprintf("%s:%d\n", file, line))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
func formatFilePath(file string) string {
	wd, err := os.Getwd()
	if err != nil {
		return file
	}

	// 将工作目录和文件路径都转为统一的正斜杠分隔格式
	normalizedWD := filepath.ToSlash(wd)
	normalizedFile := filepath.ToSlash(file)
	// 获取项目目录名称
	projectDirName := filepath.Base(normalizedWD)

	// 如果文件路径以当前工作目录开头，则返回形如 "项目目录/后续路径"
	if strings.HasPrefix(normalizedFile, normalizedWD) {
		relPath := strings.TrimPrefix(normalizedFile, normalizedWD)
		relPath = strings.TrimLeft(relPath, "/")
		return fmt.Sprintf("%s/%s", projectDirName, relPath)
	}

	parts := strings.Split(normalizedFile, "/")
	n := len(parts)
	var offset int

	if n >= 8 {
		offset = 3
	} else if n >= 1 {
		offset = 1
	}
	return strings.Join(parts[offset:], "/")
}
