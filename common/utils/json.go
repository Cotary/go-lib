package utils

import (
	"encoding/json"
	jsoniter "github.com/json-iterator/go"
	"github.com/tidwall/gjson"
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
func IsJson(str string) bool {
	var js json.RawMessage
	return NJson.Unmarshal([]byte(str), &js) == nil
}

func GValue(data gjson.Result) (any, error) {
	var res interface{}
	err := NJson.Unmarshal([]byte(data.String()), &res)
	return res, err
}
