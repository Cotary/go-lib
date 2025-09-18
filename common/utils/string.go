package utils

import (
	"crypto/rand"
	"math/big"
	"strings"
)

// AnyToInt 将任意类型转换为 int64（依赖 AnyToAny）
func AnyToInt(value interface{}) int64 {
	var res int64
	_ = AnyToAny(value, &res)
	return res
}

// AnyJoinToString 将多个值拼接成一个字符串
func AnyJoinToString(parts ...interface{}) string {
	var sb strings.Builder
	for _, v := range parts {
		sb.WriteString(AnyToString(v))
	}
	return sb.String()
}

// AnyToString 将任意类型转换为字符串（依赖 ToString）
func AnyToString(value interface{}) string {
	val, _ := ToString(value)
	return val
}

// FirstUpper 将字符串首字母转为大写
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

// GenerateCode 生成指定长度的随机码，可自定义字符集
func GenerateCode(length int, charset ...string) string {
	if length <= 0 {
		return ""
	}

	char := Mixed
	if len(charset) > 0 && charset[0] != "" {
		char = charset[0]
	}
	if char == "" {
		return ""
	}

	code := make([]byte, length)
	for i := range code {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(char))))
		if err != nil {
			return ""
		}
		code[i] = char[num.Int64()]
	}
	return string(code)
}
