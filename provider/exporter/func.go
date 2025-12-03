package exporter

import (
	"fmt"
	"strings"
	"time"

	"github.com/Cotary/go-lib/common/utils"
	"github.com/shopspring/decimal"
)

// EscapeFunc 转义函数类型
type EscapeFunc func(origin any, arg string) (any, error)

// 内置转义函数
var escapeFuncs = map[string]EscapeFunc{
	"date":   escapeDate,
	"add":    escapeAdd,
	"sub":    escapeSub,
	"mul":    escapeMul,
	"div":    escapeDiv,
	"floor":  escapeFloor,
	"format": escapeFormat,
	"enum":   escapeEnum,
}

// RegisterEscapeFunc 注册自定义转义函数
func RegisterEscapeFunc(key string, f EscapeFunc) {
	escapeFuncs[key] = f
}

// GetEscapeFunc 获取转义函数
func GetEscapeFunc(key string) (EscapeFunc, bool) {
	f, ok := escapeFuncs[key]
	return f, ok
}

func escapeDate(origin any, arg string) (any, error) {
	originVal, err := utils.AnyToAny[int64](origin)
	if err != nil {
		return nil, fmt.Errorf("date val error: %w", err)
	}
	if originVal == 0 {
		return "", nil
	}
	formatStr := "2006-01-02 15:04:05"
	if arg != "" {
		formatStr = arg
	}
	return time.Unix(originVal, 0).Format(formatStr), nil
}

func escapeAdd(origin any, arg string) (any, error) {
	originVal, err := decimal.NewFromString(utils.AnyToString(origin))
	if err != nil {
		return nil, fmt.Errorf("add origin error: %w", err)
	}
	argVal, err := decimal.NewFromString(arg)
	if err != nil {
		return nil, fmt.Errorf("add arg error: %w", err)
	}
	return originVal.Add(argVal).String(), nil
}

func escapeSub(origin any, arg string) (any, error) {
	originVal, err := decimal.NewFromString(utils.AnyToString(origin))
	if err != nil {
		return nil, fmt.Errorf("sub origin error: %w", err)
	}
	argVal, err := decimal.NewFromString(arg)
	if err != nil {
		return nil, fmt.Errorf("sub arg error: %w", err)
	}
	return originVal.Sub(argVal).String(), nil
}

func escapeMul(origin any, arg string) (any, error) {
	originVal, err := decimal.NewFromString(utils.AnyToString(origin))
	if err != nil {
		return nil, fmt.Errorf("mul origin error: %w", err)
	}
	argVal, err := decimal.NewFromString(arg)
	if err != nil {
		return nil, fmt.Errorf("mul arg error: %w", err)
	}
	return originVal.Mul(argVal).String(), nil
}

func escapeDiv(origin any, arg string) (any, error) {
	originVal, err := decimal.NewFromString(utils.AnyToString(origin))
	if err != nil {
		return nil, fmt.Errorf("div origin error: %w", err)
	}
	argVal, err := decimal.NewFromString(arg)
	if err != nil || argVal.IsZero() {
		if err == nil {
			err = fmt.Errorf("divisor is zero")
		}
		return nil, fmt.Errorf("div arg error: %w", err)
	}
	return originVal.Div(argVal).String(), nil
}

func escapeFloor(origin any, arg string) (any, error) {
	originVal, err := decimal.NewFromString(utils.AnyToString(origin))
	if err != nil {
		return nil, fmt.Errorf("floor origin error: %w", err)
	}
	return originVal.Floor().String(), nil
}

func escapeFormat(origin any, arg string) (any, error) {
	originVal, err := utils.AnyToAny[string](origin)
	if err != nil {
		return nil, fmt.Errorf("format origin error: %w", err)
	}
	return strings.ReplaceAll(arg, "%s", originVal), nil
}

func escapeEnum(origin any, arg string) (any, error) {
	originVal, err := utils.AnyToAny[string](origin)
	if err != nil {
		return nil, fmt.Errorf("enum origin error: %w", err)
	}
	args := strings.Split(arg, " ")
	for _, val := range args {
		if strings.HasPrefix(val, originVal+":") {
			parts := strings.SplitN(val, ":", 2)
			if len(parts) > 1 {
				return parts[1], nil
			}
		}
	}
	return origin, nil
}
