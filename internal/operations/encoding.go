package operations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
)

const (
	EncoderVersion    = "chab-json-v1"
	RawEncoderVersion = "raw-json-v1"
)

// CanonicalizeJSON returns deterministic JSON bytes for native operation
// requests. It sorts object keys, removes insignificant whitespace and
// preserves JSON number spelling.
func CanonicalizeJSON(raw []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		trimmed = []byte(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("request body must be valid JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf("request body must contain one JSON value")
	} else if err != io.EOF {
		return nil, fmt.Errorf("request body must contain one JSON value: %w", err)
	}
	var out bytes.Buffer
	if err := writeCanonicalJSON(&out, value); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// RequestDryRun reports whether an object request explicitly asks the server
// for dry_run:true. A malformed dry_run value fails locally so a caller does
// not accidentally turn an intended dry run into live work.
func RequestDryRun(raw []byte) (bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return false, err
	}
	value, ok := object["dry_run"]
	if !ok {
		return false, nil
	}
	switch string(bytes.TrimSpace(value)) {
	case "true":
		return true, nil
	case "false", "null":
		return false, nil
	default:
		return false, fmt.Errorf("dry_run must be a JSON boolean when present")
	}
}

func writeCanonicalJSON(out *bytes.Buffer, value any) error {
	switch v := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if v {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case string:
		data, _ := json.Marshal(v)
		out.Write(data)
	case json.Number:
		if !validJSONNumber(v.String()) {
			return fmt.Errorf("invalid JSON number %q", v.String())
		}
		out.WriteString(v.String())
	case float64:
		out.WriteString(strconv.FormatFloat(v, 'g', -1, 64))
	case []any:
		out.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeCanonicalJSON(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			keyJSON, _ := json.Marshal(key)
			out.Write(keyJSON)
			out.WriteByte(':')
			if err := writeCanonicalJSON(out, v[key]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		out.Write(data)
	}
	return nil
}

func validJSONNumber(value string) bool {
	var n json.Number = json.Number(value)
	_, err := n.Float64()
	return err == nil
}
