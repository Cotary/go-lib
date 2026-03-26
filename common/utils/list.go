package utils

import (
	"reflect"
	"strings"
)

// 以下函数已迁移至 github.com/samber/lo：
//   KeySlice  -> lo.Map
//   KeyMap    -> lo.Associate / lo.SliceToMap
//   InArray   -> lo.Contains
//   Chunk     -> lo.Chunk
//   Unique    -> lo.Uniq
//   Intersect -> lo.Intersect
//   Union     -> lo.Union
//   Difference -> lo.Difference

func Join[T any](nums []T, sep string) string {
	strList := make([]string, len(nums))
	for i, n := range nums {
		strList[i] = AnyToString(n)
	}
	return strings.Join(strList, sep)
}

// ToSlice 将一个或多个值转换为切片
func ToSlice[T any](items ...T) []T {
	return items
}

// DefaultIfZero 如果值为零值，则返回默认值
func DefaultIfZero[T comparable](value, defaultValue T) T {
	var zero T
	if value == zero {
		return defaultValue
	}
	return value
}

func DefaultIfZeroReflect[T any](value, defaultValue T) T {
	if reflect.DeepEqual(value, reflect.Zero(reflect.TypeOf(value)).Interface()) {
		return defaultValue
	}
	return value
}

// SafeSet 安全地向切片指定索引赋值（自动扩容）
func SafeSet[T any](s *[]T, index int, value T) {
	EnsureLen(s, index+1)
	(*s)[index] = value
}

// EnsureLen 扩展切片到指定长度
func EnsureLen[T any](s *[]T, length int) {
	if cap(*s) < length {
		newSlice := make([]T, length)
		copy(newSlice, *s)
		*s = newSlice
	} else {
		*s = (*s)[:length]
	}
}
