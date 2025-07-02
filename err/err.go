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

	var stackList []error
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

	var str strings.Builder
	str.WriteString("\n")
	for i, e := range stackList {
		str.WriteString(fmt.Sprintf("[%d]:%s\n", i+1, e.Error()))
		if stackErr, ok := e.(interface {
			StackTrace() errors.StackTrace
		}); ok {
			isFirstErr := len(stackList) == i+1
			for si, sf := range stackErr.StackTrace() {
				// 对于非最外层错误，跳过第一个栈信息
				if !isFirstErr && si == 0 {
					continue
				}
				pc := uintptr(sf) - 1
				fn := runtime.FuncForPC(pc)
				file, line := fn.FileLine(pc)
				// 调用辅助函数处理路径格式
				//formattedFile := formatFilePath(file) //使用 go build -trimpath
				str.WriteString(fmt.Sprintf("%s:%d\n", file, line))
			}
			str.WriteString("\n")
		}
	}
	return str.String()
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
