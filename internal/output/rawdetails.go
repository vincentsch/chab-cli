package output

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/vincentsch/chab-cli/internal/redact"
)

// structuredRawDetails converts bootstrap RawDetails into rows only when every
// value is scalar. Nested objects and arrays are skipped entirely so new server
// detail shapes do not produce noisy or misleading partial output.
func structuredRawDetails(raw json.RawMessage) map[string]any {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil
	}
	out := make(map[string]any, len(fields))
	for key, value := range fields {
		scalar, ok := decodeRawDetailScalar(value)
		if !ok {
			return nil
		}
		out[key] = scalar
	}
	return out
}

// decodeRawDetailScalar accepts the small set of values that can be rendered as
// stable one-line details in both human and JSON error output.
func decodeRawDetailScalar(raw json.RawMessage) (any, bool) {
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return nil, true
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, false
	}
	if dec.More() {
		return nil, false
	}
	switch typed := value.(type) {
	case string:
		return redact.String(typed), true
	case bool:
		return typed, true
	case json.Number:
		return typed, true
	default:
		return nil, false
	}
}

// scalarDetailText formats scalar raw details without adding JSON quotes around
// strings in human output.
func scalarDetailText(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(redact.String(strings.TrimPrefix(strings.TrimSuffix(jsonString(value), `"`), `"`)))
	}
}

func jsonString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}
