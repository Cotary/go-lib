package utils

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestStringToAndToString(t *testing.T) {
	tests := []struct {
		input    any
		expected string
	}{
		{12345, "12345"},
		{int8(123), "123"},
		{int16(12345), "12345"},
		{int32(12345), "12345"},
		{int64(12345), "12345"},
		{uint(12345), "12345"},
		{uint8(123), "123"},
		{uint16(12345), "12345"},
		{uint32(12345), "12345"},
		{uint64(12345), "12345"},
		{float32(123.45), "123.45"},
		{float64(123.45), "123.45"},
		{true, "true"},
		{"hello", "hello"},
		{[]int{1, 2, 3}, `[1,2,3]`},
		{[]byte("hello world"), "hello world"}, //字符
		{[]byte{0x01, 0x02, 0x03}, base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03})},
		{[]byte(",./"), ",./"},
		{map[string]string{"key": "value"}, `{"key":"value"}`},
		{struct{ Key string }{Key: "value"}, `{"Key":"value"}`},
		{&struct{ Key string }{Key: "value"}, `{"Key":"value"}`},
		{any(map[string]any{"key": "value"}), `{"key":"value"}`},
		{any(map[string]any{"key": json.Number("123456789123456789123456789")}), `{"key":123456789123456789123456789}`},
	}

	for _, tt := range tests {
		fmt.Println("\n", reflect.TypeOf(tt.input))
		// 测试ToString方法
		str, err := ToString(tt.input)
		fmt.Println("ToString:", str)
		if err != nil {
			t.Errorf("ToString(%v) returned error: %v", tt.input, err)
		}
		if str != tt.expected {
			t.Errorf("ToString(%v) = %v, want %v", tt.input, str, tt.expected)
		}

		// 测试StringTo方法
		target := reflect.New(reflect.TypeOf(tt.input)).Interface()
		err = StringTo(str, target)
		fmt.Println("StringTo:", reflect.ValueOf(target).Elem().Interface())
		if reflect.TypeOf(tt.input).Kind() == reflect.Slice && reflect.TypeOf(tt.input).Elem().Kind() == reflect.Uint8 {
			fmt.Println("Converted value:", string(reflect.ValueOf(target).Elem().Bytes()))
		}
		if err != nil {
			t.Errorf("StringTo(%v, %T) returned error: %v", str, target, err)
		}
		if !reflect.DeepEqual(reflect.ValueOf(target).Elem().Interface(), tt.input) {
			t.Errorf("StringTo(%v, %T) = %v, want %v", str, target, reflect.ValueOf(target).Elem().Interface(), tt.input)
		}
	}
}
