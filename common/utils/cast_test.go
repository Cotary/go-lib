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
		{[]byte("hello world"), base64.StdEncoding.EncodeToString([]byte("hello world"))},
		{[]byte("abcd"), base64.StdEncoding.EncodeToString([]byte("abcd"))},
		{[]byte{0x01, 0x02, 0x03}, base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03})},
		{[]byte(",./"), base64.StdEncoding.EncodeToString([]byte(",./"))},
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

		// 测试StringToPtr方法
		target := reflect.New(reflect.TypeOf(tt.input)).Interface()
		err = StringToPtr(str, target)
		fmt.Println("StringToPtr:", reflect.ValueOf(target).Elem().Interface())
		if reflect.TypeOf(tt.input).Kind() == reflect.Slice && reflect.TypeOf(tt.input).Elem().Kind() == reflect.Uint8 {
			fmt.Println("Converted value:", string(reflect.ValueOf(target).Elem().Bytes()))
		}
		if err != nil {
			t.Errorf("StringToPtr(%v, %T) returned error: %v", str, target, err)
		}
		if !reflect.DeepEqual(reflect.ValueOf(target).Elem().Interface(), tt.input) {
			t.Errorf("StringToPtr(%v, %T) = %v, want %v", str, target, reflect.ValueOf(target).Elem().Interface(), tt.input)
		}
	}
}

// TestNamedTypes 验证命名基础类型不再被 ToString 拒绝。
func TestNamedTypes(t *testing.T) {
	type namedString string
	type namedInt int64
	type namedUint uint32
	type namedFloat float64
	type namedBool bool
	type namedBytes []byte

	if got, err := ToString(namedString("hello")); err != nil || got != "hello" {
		t.Fatalf("named string ToString: %q %v", got, err)
	}
	if got, err := StringTo[namedString]("hello"); err != nil || got != namedString("hello") {
		t.Fatalf("named string StringTo: %q %v", got, err)
	}
	if got, err := ToString(namedInt(-42)); err != nil || got != "-42" {
		t.Fatalf("named int ToString: %q %v", got, err)
	}
	if got, err := StringTo[namedInt]("-42"); err != nil || got != namedInt(-42) {
		t.Fatalf("named int StringTo: %d %v", got, err)
	}
	if got, err := ToString(namedUint(42)); err != nil || got != "42" {
		t.Fatalf("named uint ToString: %q %v", got, err)
	}
	if got, err := StringTo[namedUint]("42"); err != nil || got != namedUint(42) {
		t.Fatalf("named uint StringTo: %d %v", got, err)
	}
	if got, err := ToString(namedFloat(1.5)); err != nil || got != "1.5" {
		t.Fatalf("named float ToString: %q %v", got, err)
	}
	if got, err := StringTo[namedFloat]("1.5"); err != nil || got != namedFloat(1.5) {
		t.Fatalf("named float StringTo: %v %v", got, err)
	}
	if got, err := ToString(namedBool(true)); err != nil || got != "true" {
		t.Fatalf("named bool ToString: %q %v", got, err)
	}
	if got, err := StringTo[namedBool]("true"); err != nil || got != namedBool(true) {
		t.Fatalf("named bool StringTo: %t %v", got, err)
	}
	encoded, err := ToString(namedBytes("abcd"))
	if err != nil || encoded != base64.StdEncoding.EncodeToString([]byte("abcd")) {
		t.Fatalf("named bytes ToString: %q %v", encoded, err)
	}
	decoded, err := StringTo[namedBytes](encoded)
	if err != nil || string(decoded) != "abcd" {
		t.Fatalf("named bytes StringTo: %q %v", decoded, err)
	}
}

// TestBytesStrictBase64 验证 []byte 只走 base64，不再回退 raw。
func TestBytesStrictBase64(t *testing.T) {
	encoded, err := ToString([]byte("abcd"))
	if err != nil {
		t.Fatal(err)
	}
	if encoded != base64.StdEncoding.EncodeToString([]byte("abcd")) {
		t.Fatalf("encoded: %q", encoded)
	}
	decoded, err := StringTo[[]byte](encoded)
	if err != nil || string(decoded) != "abcd" {
		t.Fatalf("decoded: %q %v", decoded, err)
	}

	binary := []byte{0x00, 0xff, 0x10}
	encoded, err = ToString(binary)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = StringTo[[]byte](encoded)
	if err != nil || !reflect.DeepEqual(decoded, binary) {
		t.Fatalf("binary round trip: %v %v", decoded, err)
	}

	if _, err := StringTo[[]byte]("not base64!"); err == nil {
		t.Fatal("invalid base64 accepted by StringTo")
	}
	var target []byte
	if err := StringToPtr("not base64!", &target); err == nil {
		t.Fatal("invalid base64 accepted by StringToPtr")
	}
}

// TestAnyToAnyBytes 验证 AnyToAny 的 []byte 严格路径。
func TestAnyToAnyBytes(t *testing.T) {
	data, err := AnyToAny[[]byte]([]byte("abcd"))
	if err != nil || string(data) != "abcd" {
		t.Fatalf("same type: %q %v", data, err)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("abcd"))
	data, err = AnyToAny[[]byte](encoded)
	if err != nil || string(data) != "abcd" {
		t.Fatalf("base64 string: %q %v", data, err)
	}
	text, err := AnyToAny[string]([]byte("abcd"))
	if err != nil || text != encoded {
		t.Fatalf("bytes to string: %q %v", text, err)
	}
}

// TestNilPointer 验证 nil 指针的 JSON null 往返。
func TestNilPointer(t *testing.T) {
	type payload struct {
		Value int `json:"value"`
	}
	var input *payload
	text, err := ToString(input)
	if err != nil || text != "null" {
		t.Fatalf("nil pointer ToString: %q %v", text, err)
	}
	var output *payload
	if err := StringToPtr(text, &output); err != nil || output != nil {
		t.Fatalf("nil pointer StringToPtr: %+v %v", output, err)
	}
}
