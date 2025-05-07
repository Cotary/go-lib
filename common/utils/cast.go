package utils

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"strconv"
)

func AnyToAny(value any, target any) error {
	str, err := ToString(value)
	if err != nil {
		return err
	}
	return StringTo(str, target)
}

func StringTo(value string, target any) error {
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
		err := StringTo(value, ptrVal.Interface())
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

// ToString 这里不支持String()的方法，因为可能没有StringTo的方法
func ToString(value any) (string, error) {
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, v.Type().Bits()), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.String:
		return v.String(), nil
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			if isTextData(v.Bytes()) {
				return string(v.Bytes()), nil
			} else {
				return base64.StdEncoding.EncodeToString(v.Bytes()), nil
			}
		} else {
			bytes, err := NJson.Marshal(value)
			if err != nil {
				return "", err
			}
			return string(bytes), nil
		}
	case reflect.Map, reflect.Struct, reflect.Ptr, reflect.Interface:
		bytes, err := NJson.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(bytes), nil
	default:
		return "", fmt.Errorf("unsupported type: %s", v.Kind())
	}
}

func isTextData(data []byte) bool {
	for _, b := range data {
		if b < 32 || b > 126 {
			// 如果发现非ASCII文本字符，判断为二进制数据
			return false
		}
	}
	return true
}
