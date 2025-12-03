package utils

import (
	"encoding/json"
	jsoniter "github.com/json-iterator/go"
)

var NJson = jsoniter.Config{
	EscapeHTML:             true,
	SortMapKeys:            true,
	ValidateJsonRawMessage: true,
	UseNumber:              true, //使用高精度Number
}.Froze()

func Json(data interface{}) string {
	byteUser, _ := NJson.Marshal(data)
	return string(byteUser)
}
func IsJson(data []byte) bool {
	return NJson.Valid(data)
}

func JsonRaw(data []byte) json.RawMessage {
	return data
}

// Unmarshal 返回解析后的 T 类型的值。
func Unmarshal[T any](data []byte) (T, error) {
	// 创建一个 T 类型的零值
	var result T
	err := NJson.Unmarshal(data, &result)
	if err != nil {
		return result, err
	}
	// 返回解析后的值
	return result, nil
}
