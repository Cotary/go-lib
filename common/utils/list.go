package utils

import "reflect"

// MapSlice 提取结构体切片中的某个字段，返回字段值的切片
func MapSlice[T any, U any](src []T, mapper func(T) U) []U {
	result := make([]U, len(src))
	for i, v := range src {
		result[i] = mapper(v)
	}
	return result
}

// KeyBy 将结构体切片按某个字段作为键，返回键值对映射
func KeyBy[T any, K comparable](src []T, keySelector func(T) K) map[K]T {
	result := make(map[K]T, len(src))
	for _, v := range src {
		result[keySelector(v)] = v
	}
	return result
}

// InArray 判断某个值是否在切片中
func InArray[T comparable](val T, arr []T) bool {
	for _, item := range arr {
		if item == val {
			return true
		}
	}
	return false
}

// ToSlice 将一个或多个值转换为切片
func ToSlice[T any](items ...T) []T {
	return items
}

// DefaultIfZero 如果值为零值，则返回默认值
func DefaultIfZero[T any](value, defaultValue T) T {
	if reflect.DeepEqual(value, reflect.Zero(reflect.TypeOf(value)).Interface()) {
		return defaultValue
	}
	return value
}

// Chunk 将切片按指定大小分割
func Chunk[T any](slice []T, size int) [][]T {
	if size <= 0 {
		return nil
	}
	result := make([][]T, 0, (len(slice)+size-1)/size)
	for i := 0; i < len(slice); i += size {
		end := i + size
		if end > len(slice) {
			end = len(slice)
		}
		result = append(result, slice[i:end])
	}
	return result
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

// Unique 去重
func Unique[T comparable](s []T) []T {
	seen := make(map[T]struct{}, len(s))
	result := make([]T, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

// Intersect 交集
func Intersect[T comparable](a, b []T) []T {
	m := make(map[T]struct{}, len(a))
	for _, item := range a {
		m[item] = struct{}{}
	}
	out := make([]T, 0)
	for _, item := range b {
		if _, ok := m[item]; ok {
			out = append(out, item)
		}
	}
	return out
}

// Union 并集
func Union[T comparable](a, b []T) []T {
	m := make(map[T]struct{}, len(a)+len(b))
	for _, item := range a {
		m[item] = struct{}{}
	}
	for _, item := range b {
		m[item] = struct{}{}
	}
	out := make([]T, 0, len(m))
	for item := range m {
		out = append(out, item)
	}
	return out
}

// Difference 差集（a - b）
func Difference[T comparable](a, b []T) []T {
	m := make(map[T]struct{}, len(b))
	for _, item := range b {
		m[item] = struct{}{}
	}
	out := make([]T, 0)
	for _, item := range a {
		if _, ok := m[item]; !ok {
			out = append(out, item)
		}
	}
	return out
}
