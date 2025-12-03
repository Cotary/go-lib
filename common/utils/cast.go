package utils

import (
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

// ================== AnyToAny 系列 ==================

// AnyToAny 泛型版本，常见基础类型零反射
func AnyToAny[T any](value any) (T, error) {
	var zero T

	// 快路径：如果源和目标类型相同，直接返回
	if v, ok := value.(T); ok {
		return v, nil
	}

	// 源转字符串
	str, err := ToString(value)
	if err != nil {
		return zero, err
	}

	// 字符串转目标（泛型版本）
	return StringTo[T](str)
}

// AnyToAnyPtr 反射版本，支持复杂类型和动态目标类型
func AnyToAnyPtr(value any, target any) error {
	str, err := ToString(value)
	if err != nil {
		return err
	}
	return StringToPtr(str, target)
}

// ================== StringTo 系列 ==================

// StringTo 泛型版本，基础类型使用类型断言，零反射
func StringTo[T any](value string) (T, error) {
	var zero T
	result, err := stringToFast(value, zero)
	if err != nil {
		if errors.Is(err, errNeedReflect) {
			// 快路径失败，回退到反射
			err = StringToPtr(value, &zero)
			return zero, err
		}
		return zero, err
	}
	if v, ok := result.(T); ok {
		return v, nil
	}
	// 类型断言失败，回退到反射
	err = StringToPtr(value, &zero)
	return zero, err
}

// stringToFast 类型断言快路径，处理常见基础类型
func stringToFast(value string, target any) (any, error) {
	switch target.(type) {
	case string:
		return value, nil
	case int:
		return strconv.Atoi(value)
	case int8:
		v, err := strconv.ParseInt(value, 10, 8)
		return int8(v), err
	case int16:
		v, err := strconv.ParseInt(value, 10, 16)
		return int16(v), err
	case int32:
		v, err := strconv.ParseInt(value, 10, 32)
		return int32(v), err
	case int64:
		return strconv.ParseInt(value, 10, 64)
	case uint:
		v, err := strconv.ParseUint(value, 10, 0)
		return uint(v), err
	case uint8:
		v, err := strconv.ParseUint(value, 10, 8)
		return uint8(v), err
	case uint16:
		v, err := strconv.ParseUint(value, 10, 16)
		return uint16(v), err
	case uint32:
		v, err := strconv.ParseUint(value, 10, 32)
		return uint32(v), err
	case uint64:
		return strconv.ParseUint(value, 10, 64)
	case float32:
		v, err := strconv.ParseFloat(value, 32)
		return float32(v), err
	case float64:
		return strconv.ParseFloat(value, 64)
	case bool:
		return strconv.ParseBool(value)
	case []byte:
		bytes, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			// 不是 base64 编码的情况，直接转换为 []byte
			return []byte(value), nil
		}
		return bytes, nil
	}
	// 返回特殊错误标记，表示需要回退到反射
	return nil, errNeedReflect
}

var errNeedReflect = fmt.Errorf("need reflect")

// StringToPtr 反射版本，支持复杂类型
func StringToPtr(value string, target any) error {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("target must be a non-nil pointer")
	}
	v = v.Elem()
	switch v.Kind() {
	case reflect.Int:
		intVal, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		v.SetInt(int64(intVal))
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intVal, err := strconv.ParseInt(value, 10, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetInt(intVal)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintVal, err := strconv.ParseUint(value, 10, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetUint(uintVal)
	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(value, v.Type().Bits())
		if err != nil {
			return err
		}
		v.SetFloat(floatVal)
	case reflect.Bool:
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		v.SetBool(boolVal)
	case reflect.String:
		v.SetString(value)
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			bytes, err := base64.StdEncoding.DecodeString(value)
			if err != nil {
				// 不是 base64 编码的情况，直接转换为 []byte
				v.SetBytes([]byte(value))
			} else {
				// base64 编码的情况，解码为 []byte
				v.SetBytes(bytes)
			}
		} else {
			err := NJson.Unmarshal([]byte(value), target)
			if err != nil {
				return err
			}
		}
	case reflect.Map, reflect.Struct:
		err := NJson.Unmarshal([]byte(value), target)
		if err != nil {
			return err
		}
	case reflect.Ptr:
		ptrVal := reflect.New(v.Type().Elem())
		err := StringToPtr(value, ptrVal.Interface())
		if err != nil {
			return err
		}
		v.Set(ptrVal)
	case reflect.Interface:
		var ifaceVal any
		err := NJson.Unmarshal([]byte(value), &ifaceVal)
		if err != nil {
			return err
		}
		v.Set(reflect.ValueOf(ifaceVal))
	default:
		return fmt.Errorf("unsupported type: %s", v.Kind())
	}
	return nil
}

// ================== ToString ==================

// ToString 这里不支持String()的方法，因为可能没有StringTo的方法
// 优化：基础类型使用类型断言快路径，复杂类型回退到反射
func ToString(value any) (string, error) {
	// 快路径：类型断言处理常见基础类型
	switch v := value.(type) {
	case string:
		return v, nil
	case int:
		return strconv.Itoa(v), nil
	case int8:
		return strconv.FormatInt(int64(v), 10), nil
	case int16:
		return strconv.FormatInt(int64(v), 10), nil
	case int32:
		return strconv.FormatInt(int64(v), 10), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case uint:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(v), 10), nil
	case uint64:
		return strconv.FormatUint(v, 10), nil
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	case []byte:
		if isTextData(v) {
			return string(v), nil
		}
		return base64.StdEncoding.EncodeToString(v), nil
	case nil:
		return "", nil
	}

	// 慢路径：反射处理复杂类型
	return toStringReflect(value)
}

// toStringReflect 反射版本，处理 Slice/Map/Struct/Ptr/Interface 等复杂类型
func toStringReflect(value any) (string, error) {
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			if isTextData(v.Bytes()) {
				return string(v.Bytes()), nil
			}
			return base64.StdEncoding.EncodeToString(v.Bytes()), nil
		}
		fallthrough
	case reflect.Map, reflect.Struct, reflect.Ptr, reflect.Interface:
		bytes, err := NJson.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(bytes), nil
	default:
		return "", fmt.Errorf("unsupported type: %T", value)
	}
}

// ================== 辅助函数 ==================

func isTextData(data []byte) bool {
	for _, b := range data {
		if b < 32 || b > 126 {
			// 如果发现非ASCII文本字符，判断为二进制数据
			return false
		}
	}
	return true
}
