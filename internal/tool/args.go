package tool

import (
	"bytes"
	"encoding/json"
)

func decodeArgs[T any](raw json.RawMessage) (T, error) {
	var v T
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	err := dec.Decode(&v)
	return v, err
}
