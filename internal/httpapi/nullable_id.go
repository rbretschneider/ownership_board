package httpapi

import (
	"bytes"
	"encoding/json"
)

// nullableID decodes an optional JSON id where null is meaningful: absent
// leaves IsSet false, null sets it with value zero (the domain's "none").
// It must be a value field, not a pointer, because encoding/json turns null
// into a nil pointer without ever calling UnmarshalJSON.
type nullableID struct {
	value int64
	isSet bool
}

// UnmarshalJSON implements json.Unmarshaler.
func (n *nullableID) UnmarshalJSON(data []byte) error {
	n.isSet = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		n.value = 0
		return nil
	}
	return json.Unmarshal(data, &n.value)
}
