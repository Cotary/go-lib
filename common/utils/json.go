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
