package utils

import (
	"encoding/json"
	"github.com/spf13/cast"
	"strings"
)

func Json(data interface{}) string {
	byteUser, _ := json.Marshal(data)
	return string(byteUser)
}

func AnyToInt(value interface{}) int64 {
	return cast.ToInt64(value)
}

func AnyToIntArray[T any](value []T) []int64 {
	var res []int64
	for _, v := range value {
		res = append(res, cast.ToInt64(v))
	}
	return res

}

func AnyJoinToString(data ...interface{}) string {
	var str string
	for _, v := range data {
		str = str + AnyToString(v)
	}
	return str
}

func AnyToString(value interface{}) string {
	return cast.ToString(value)
}

func FirstUpper(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
