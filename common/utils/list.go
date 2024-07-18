package utils

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
func RemoveDuplicates[T comparable](s []T) []T {
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
