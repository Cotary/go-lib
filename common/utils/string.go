package utils

import (
	"crypto/rand"
	"math/big"
	"strings"
)

func AnyToInt(value interface{}) (res int64) {
	_ = AnyToAny(value, &res)
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
	val, _ := ToString(value)
	return val
}

func FirstUpper(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

const (
	Num     = "23456789"                 // 去除 0 和 1
	Letters = "ABCDEFGHJKLMNPQRSTUVWXYZ" // 去除 O, I, l
	Mixed   = Num + Letters
)

func GenerateCode(length int, charset ...string) string {
	code := make([]byte, length)
	char := Mixed
	if len(charset) > 0 {
		char = charset[0]
	}
	if char == "" {
		return ""
	}
	for i := range code {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(char))))
		if err != nil {
			return ""
		}
		code[i] = char[num.Int64()]
	}
	return string(code)
}
