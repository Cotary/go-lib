package utils

import "reflect"

// ListFields 提取结构体切片中的某个字段，返回字段值的切片
func ListFields[T any, U any](s []T, f func(T) U) []U {
	result := make([]U, len(s))
	for i, v := range s {
		result[i] = f(v)
	}
	return result
}

// ArrayColumn 将结构体切片中的某个字段作为键，返回键值对的映射
func ArrayColumn[T any, U comparable](s []T, f func(T) U) map[U]T {
	result := make(map[U]T, len(s))
	for _, v := range s {
		result[f(v)] = v
	}
	return result
}

// InArray 判断某个值是否在切片中
func InArray[T comparable](val T, array []T) bool {
	for _, item := range array {
		if item == val {
			return true
		}
	}
	return false
}

// ToSlice 将单个值转换为切片
func ToSlice[T any](item ...T) []T {
	return item
}

// DefaultIfEmpty 如果值为空，则返回默认值
func DefaultIfEmpty[T any](value, defaultValue T) T {
	if reflect.DeepEqual(value, reflect.Zero(reflect.TypeOf(value)).Interface()) {
		return defaultValue
	}
	return value
}

// SplitSlice 将切片按指定大小分割
func SplitSlice[T any](slice []T, size int) [][]T {
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

// SafeSliceAdd 安全地向切片中添加元素
func SafeSliceAdd[T any](s *[]T, key int, value T) {
	ExtendSlice(s, key+1)
	(*s)[key] = value
}

// ExtendSlice 扩展切片到指定长度
func ExtendSlice[T any](s *[]T, length int) {
	if cap(*s) < length {
		newSlice := make([]T, length)
		copy(newSlice, *s)
		*s = newSlice
	} else {
		*s = (*s)[:length]
	}
}

// ListUnique 去重
func ListUnique[T comparable](s []T) []T {
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

// Intersection 交集
func Intersection[T comparable](a, b []T) []T {
	m := make(map[T]struct{}, len(a))
	for _, item := range a {
		m[item] = struct{}{}
	}
	var out []T
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

// Difference 差集
func Difference[T comparable](a, b []T) []T {
	m := make(map[T]struct{}, len(b))
	for _, item := range b {
		m[item] = struct{}{}
	}
	var out []T
	for _, item := range a {
		if _, ok := m[item]; !ok {
			out = append(out, item)
		}
	}
	return out
}
