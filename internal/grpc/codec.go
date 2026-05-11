package grpcsvc

import (
	"encoding/json"
	"fmt"

	"google.golang.org/grpc/encoding"
)

const CodecName = "json"

type jsonCodec struct{}

func (jsonCodec) Name() string { return CodecName }

func (jsonCodec) Marshal(v interface{}) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	return b, nil
}

func (jsonCodec) Unmarshal(data []byte, v interface{}) error {
	if v == nil {
		return nil
	}
	return json.Unmarshal(data, v)
}

func RegisterCodec() {
	encoding.RegisterCodec(jsonCodec{})
}
