package utils

import "reflect"

func ListField[T any, U any](s []T, f func(T) U) []U {
	var result []U
	for _, v := range s {
		field := f(v)
		result = append(result, field)
	}
	return result
}
func DefaultIfEmpty[T any](value, defaultValue T) T {
	// Check if the value is the zero value of its type
	if reflect.DeepEqual(value, reflect.Zero(reflect.TypeOf(value)).Interface()) {
		return defaultValue
	}
	return value
}
func ExtendSlice[T any](s *[]T, length int) {
	for len(*s) < length {
		var zero T
		*s = append(*s, zero)
	}
}

func SafeSliceAdd[T any](s *[]T, key int, value T) {
	ExtendSlice(s, key+1)
	(*s)[key] = value
}

// 去重
func ListUnique[T comparable](s []T) []T {
	var seen = make(map[T]bool)
	var result []T
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

// 交集
func Intersection[T comparable](a, b []T) (out []T) {
	m := make(map[T]bool)
	for _, item := range a {
		m[item] = true
	}
	for _, item := range b {
		if m[item] {
			out = append(out, item)
		}
	}
	return
}

// 并集
func Union[T comparable](a, b []T) (out []T) {
	m := make(map[T]bool)
	for _, item := range a {
		if !m[item] {
			out = append(out, item)
			m[item] = true
		}
	}
	for _, item := range b {
		if !m[item] {
			out = append(out, item)
			m[item] = true
		}
	}
	return
}

// 差集
func Difference[T comparable](a, b []T) (out []T) {
	m := make(map[T]bool)
	for _, item := range b {
		m[item] = true
	}
	for _, item := range a {
		if !m[item] {
			out = append(out, item)
		}
	}
	return
}
