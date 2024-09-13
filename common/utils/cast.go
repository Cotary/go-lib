package utils

import (
	"encoding/json"
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
	case reflect.Int8:
		intVal, err := strconv.ParseInt(value, 10, 8)
		if err != nil {
			return err
		}
		v.SetInt(intVal)
	case reflect.Int16:
		intVal, err := strconv.ParseInt(value, 10, 16)
		if err != nil {
			return err
		}
		v.SetInt(intVal)
	case reflect.Int32:
		intVal, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return err
		}
		v.SetInt(intVal)
	case reflect.Int64:
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(intVal)
	case reflect.Uint:
		uintVal, err := strconv.ParseUint(value, 10, 0)
		if err != nil {
			return err
		}
		v.SetUint(uintVal)
	case reflect.Uint8:
		uintVal, err := strconv.ParseUint(value, 10, 8)
		if err != nil {
			return err
		}
		v.SetUint(uintVal)
	case reflect.Uint16:
		uintVal, err := strconv.ParseUint(value, 10, 16)
		if err != nil {
			return err
		}
		v.SetUint(uintVal)
	case reflect.Uint32:
		uintVal, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return err
		}
		v.SetUint(uintVal)
	case reflect.Uint64:
		uintVal, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		v.SetUint(uintVal)
	case reflect.Float32:
		floatVal, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return err
		}
		v.SetFloat(floatVal)
	case reflect.Float64:
		floatVal, err := strconv.ParseFloat(value, 64)
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
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct:
		err := json.Unmarshal([]byte(value), target)
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
		err := json.Unmarshal([]byte(value), &ifaceVal)
		if err != nil {
			return err
		}
		v.Set(reflect.ValueOf(ifaceVal))
	default:
		return fmt.Errorf("unsupported type: %s", v.Kind())
	}
	return nil
}

func ToString(value any) (string, error) {
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Float32:
		return strconv.FormatFloat(v.Float(), 'f', -1, 32), nil
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.String:
		return v.String(), nil
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct, reflect.Ptr, reflect.Interface:
		bytes, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(bytes), nil
	default:
		return "", fmt.Errorf("unsupported type: %s", v.Kind())
	}
}
