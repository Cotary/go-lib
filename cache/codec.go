package cache

import (
	"encoding/json"

	"github.com/Cotary/go-lib/common/utils"
)

// Codec defines how values are serialized for storage in Redis.
type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

var (
	JsonCodec    Codec = jsonIterCodec{}
	StdJsonCodec Codec = stdJsonCodec{}
)

// jsonIterCodec uses the project's json-iterator configuration (utils.NJson).
type jsonIterCodec struct{}

func (jsonIterCodec) Marshal(v any) ([]byte, error) {
	return utils.NJson.Marshal(v)
}

func (jsonIterCodec) Unmarshal(data []byte, v any) error {
	return utils.NJson.Unmarshal(data, v)
}

// stdJsonCodec uses the standard library encoding/json.
type stdJsonCodec struct{}

func (stdJsonCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (stdJsonCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
