package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/vincentsch/chab-cli/internal/redact"
)

// jsonNodeKind identifies the JSON token shape stored in jsonNode.
type jsonNodeKind uint8

const (
	jsonNullNode jsonNodeKind = iota
	jsonStringNode
	jsonNumberNode
	jsonBooleanNode
	jsonArrayNode
	jsonObjectNode
)

// jsonNode is an ordered JSON tree. Numbers stay as their original text and
// object members stay in source order, which ordinary map decoding would lose.
type jsonNode struct {
	kind    jsonNodeKind
	text    string
	boolean bool
	array   []*jsonNode
	object  []jsonMember
}

// jsonMember keeps the source key for deterministic collision handling and a
// separate emitted key that may be redacted or suffixed.
type jsonMember struct {
	originalKey string
	key         string
	value       *jsonNode
}

// structuredJSONBytes encodes and structurally redacts a JSON value while
// retaining the encoded document's object-member order and exact numbers.
func structuredJSONBytes(value any, indented bool) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	node, err := decodeJSONNode(raw)
	if err != nil {
		return nil, err
	}
	redactJSONNode(node)

	var compact bytes.Buffer
	if err := writeJSONNode(&compact, node); err != nil {
		return nil, err
	}
	if !indented {
		return compact.Bytes(), nil
	}
	var out bytes.Buffer
	if err := json.Indent(&out, compact.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// decodeJSONNode reads exactly one JSON value and rejects a trailing document.
func decodeJSONNode(raw []byte) (*jsonNode, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	node, err := readJSONNode(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			err = errors.New("invalid JSON after top-level value")
		}
		return nil, err
	}
	return node, nil
}

// readJSONNode recursively builds the ordered tree from the decoder's token
// stream without converting numbers to floating point values.
func readJSONNode(dec *json.Decoder) (*jsonNode, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch typed := token.(type) {
	case nil:
		return &jsonNode{kind: jsonNullNode}, nil
	case string:
		return &jsonNode{kind: jsonStringNode, text: typed}, nil
	case json.Number:
		return &jsonNode{kind: jsonNumberNode, text: typed.String()}, nil
	case bool:
		return &jsonNode{kind: jsonBooleanNode, boolean: typed}, nil
	case json.Delim:
		switch typed {
		case '[':
			node := &jsonNode{kind: jsonArrayNode, array: make([]*jsonNode, 0)}
			for dec.More() {
				child, err := readJSONNode(dec)
				if err != nil {
					return nil, err
				}
				node.array = append(node.array, child)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return node, nil
		case '{':
			node := &jsonNode{kind: jsonObjectNode, object: make([]jsonMember, 0)}
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errors.New("JSON object key is not a string")
				}
				child, err := readJSONNode(dec)
				if err != nil {
					return nil, err
				}
				node.object = append(node.object, jsonMember{originalKey: key, key: key, value: child})
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return node, nil
		}
	}
	return nil, errors.New("unsupported JSON token")
}

// redactJSONNode redacts every string value and assigns all object keys as a
// group so key collisions can be resolved without dropping any member.
func redactJSONNode(node *jsonNode) {
	if node == nil {
		return
	}
	switch node.kind {
	case jsonStringNode:
		node.text = redact.String(node.text)
	case jsonArrayNode:
		for _, child := range node.array {
			redactJSONNode(child)
		}
	case jsonObjectNode:
		keys := make([]string, len(node.object))
		for index := range node.object {
			keys[index] = node.object[index].originalKey
			redactJSONNode(node.object[index].value)
		}
		assigned := assignRedactedKeys(keys)
		for index := range node.object {
			node.object[index].key = assigned[index]
		}
	}
}

// assignRedactedKeys assigns collision suffixes in lexical raw-key order but
// returns the names at their original positions for order-preserving emission.
func assignRedactedKeys(keys []string) []string {
	indices := make([]int, len(keys))
	for index := range keys {
		indices[index] = index
	}
	sort.SliceStable(indices, func(i, j int) bool {
		return keys[indices[i]] < keys[indices[j]]
	})

	result := make([]string, len(keys))
	used := make(map[string]struct{}, len(keys))
	for _, index := range indices {
		base := redact.String(keys[index])
		candidate := base
		for suffix := 2; ; suffix++ {
			if _, exists := used[candidate]; !exists {
				break
			}
			candidate = base + "~" + strconv.Itoa(suffix)
		}
		used[candidate] = struct{}{}
		result[index] = candidate
	}
	return result
}

// writeJSONNode emits compact JSON from the ordered tree. Indentation is a
// separate final step so compact errors and stable JSON share identical logic.
func writeJSONNode(out *bytes.Buffer, node *jsonNode) error {
	if node == nil {
		out.WriteString("null")
		return nil
	}
	switch node.kind {
	case jsonNullNode:
		out.WriteString("null")
	case jsonStringNode:
		return writeJSONString(out, node.text)
	case jsonNumberNode:
		out.WriteString(node.text)
	case jsonBooleanNode:
		out.WriteString(strconv.FormatBool(node.boolean))
	case jsonArrayNode:
		out.WriteByte('[')
		for index, child := range node.array {
			if index > 0 {
				out.WriteByte(',')
			}
			if err := writeJSONNode(out, child); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case jsonObjectNode:
		out.WriteByte('{')
		for index, member := range node.object {
			if index > 0 {
				out.WriteByte(',')
			}
			if err := writeJSONString(out, member.key); err != nil {
				return err
			}
			out.WriteByte(':')
			if err := writeJSONNode(out, member.value); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	}
	return nil
}

func writeJSONString(out *bytes.Buffer, value string) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	out.Write(EscapeJSONC1Controls(encoded))
	return nil
}

// EscapeJSONC1Controls replaces raw DEL and UTF-8 C1 control runes in encoded
// JSON with equivalent escapes so terminals cannot interpret machine output.
func EscapeJSONC1Controls(encoded []byte) []byte {
	var out []byte
	for offset := 0; offset < len(encoded); {
		r, size := utf8.DecodeRune(encoded[offset:])
		if r == '\u007f' || (r >= '\u0080' && r <= '\u009f') {
			if out == nil {
				out = make([]byte, 0, len(encoded)+6)
				out = append(out, encoded[:offset]...)
			}
			const hex = "0123456789abcdef"
			out = append(out, '\\', 'u', '0', '0', hex[byte(r)>>4], hex[byte(r)&0x0f])
		} else if out != nil {
			out = append(out, encoded[offset:offset+size]...)
		}
		offset += size
	}
	if out == nil {
		return encoded
	}
	return out
}

// RedactJSONValue recursively redacts a decoded JSON-like value. Object key
// collisions are retained with deterministic suffixes so callers can safely
// marshal every original value.
func RedactJSONValue(value any) any {
	switch typed := value.(type) {
	case string:
		return redact.String(typed)
	case []any:
		for index := range typed {
			typed[index] = RedactJSONValue(typed[index])
		}
		return typed
	case map[string]any:
		keys := make([]string, 0, len(typed))
		values := make(map[string]any, len(typed))
		for key, item := range typed {
			keys = append(keys, key)
			values[key] = item
		}
		sort.Strings(keys)
		assigned := assignRedactedKeys(keys)
		// Rebuild the map only after every emitted key is known; assigning in
		// place could overwrite a value when two raw keys redact identically.
		for key := range typed {
			delete(typed, key)
		}
		for index, key := range keys {
			typed[assigned[index]] = RedactJSONValue(values[key])
		}
		return typed
	default:
		return value
	}
}
