package redact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
)

// TransformOrderedJSON applies transform to every semantic scalar value and
// object key in exactly one JSON value. Object membership and order, duplicate
// members, and array order are preserved in compact output. Numbers, Booleans,
// and null retain their exact text and primitive type unless transform changes
// that token, in which case the transformed value is emitted as a JSON string.
func TransformOrderedJSON(raw []byte, transform func(string) string) ([]byte, error) {
	if transform == nil {
		return nil, fmt.Errorf("transform ordered JSON: transform must not be nil")
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	root, err := decodeOrderedJSONValue(decoder)
	if err != nil {
		return nil, fmt.Errorf("transform ordered JSON: %w", err)
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("transform ordered JSON: trailing input: %w", err)
		}
		return nil, fmt.Errorf("transform ordered JSON: trailing JSON token %v", token)
	}

	transformOrderedJSONValues(root, transform)
	var out bytes.Buffer
	if err := writeOrderedJSON(&out, root); err != nil {
		return nil, fmt.Errorf("transform ordered JSON: %w", err)
	}
	return out.Bytes(), nil
}

type orderedJSONKind uint8

const (
	orderedJSONObject orderedJSONKind = iota
	orderedJSONArray
	orderedJSONString
	orderedJSONNumber
	orderedJSONBool
	orderedJSONNull
)

type orderedJSONMember struct {
	key      string // decoded source key used for deterministic ordering
	emitKey  string // transformed, collision-free key written to output
	value    *orderedJSONNode
	position int // source position used to keep equal keys stable
}

// orderedJSONNode is a JSON value decoded without maps. Keeping object members
// in a slice preserves their source order and duplicates, while text holds
// exact number tokens as well as decoded strings.
type orderedJSONNode struct {
	kind    orderedJSONKind
	members []orderedJSONMember
	items   []*orderedJSONNode
	text    string
	boolean bool
}

// decodeOrderedJSONValue builds the ordered tree one decoder token at a time.
// UseNumber on the caller's decoder keeps large and precisely formatted
// numbers from being converted through float64.
func decodeOrderedJSONValue(decoder *json.Decoder) (*orderedJSONNode, error) {
	token, err := decoder.Token()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("input is empty")
		}
		return nil, fmt.Errorf("decode value: %w", err)
	}

	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			node := &orderedJSONNode{kind: orderedJSONObject}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, fmt.Errorf("decode object key: %w", err)
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("object key has unsupported token type %T", keyToken)
				}
				child, err := decodeOrderedJSONValue(decoder)
				if err != nil {
					return nil, err
				}
				node.members = append(node.members, orderedJSONMember{
					key:      key,
					emitKey:  key,
					value:    child,
					position: len(node.members),
				})
			}
			end, err := decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("decode object end: %w", err)
			}
			if end != json.Delim('}') {
				return nil, fmt.Errorf("object ended with unsupported token %v", end)
			}
			return node, nil
		case '[':
			node := &orderedJSONNode{kind: orderedJSONArray}
			for decoder.More() {
				child, err := decodeOrderedJSONValue(decoder)
				if err != nil {
					return nil, err
				}
				node.items = append(node.items, child)
			}
			end, err := decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("decode array end: %w", err)
			}
			if end != json.Delim(']') {
				return nil, fmt.Errorf("array ended with unsupported token %v", end)
			}
			return node, nil
		default:
			return nil, fmt.Errorf("unsupported delimiter %q", value)
		}
	case string:
		return &orderedJSONNode{kind: orderedJSONString, text: value}, nil
	case json.Number:
		return &orderedJSONNode{kind: orderedJSONNumber, text: value.String()}, nil
	case bool:
		return &orderedJSONNode{kind: orderedJSONBool, boolean: value}, nil
	case nil:
		return &orderedJSONNode{kind: orderedJSONNull}, nil
	default:
		return nil, fmt.Errorf("unsupported token type %T", token)
	}
}

// transformOrderedJSONValues transforms scalar values recursively and assigns
// every object member a unique output key. Member values stay at their source
// positions even when their transformed keys collide.
func transformOrderedJSONValues(node *orderedJSONNode, transform func(string) string) {
	switch node.kind {
	case orderedJSONObject:
		order := make([]int, len(node.members))
		for index := range node.members {
			order[index] = index
			transformOrderedJSONValues(node.members[index].value, transform)
		}
		sort.SliceStable(order, func(i, j int) bool {
			left := node.members[order[i]]
			right := node.members[order[j]]
			if left.key == right.key {
				return left.position < right.position
			}
			return left.key < right.key
		})
		// Allocate names in lexical source-key order so suffixes do not depend on
		// the order in which colliding members happened to appear in the JSON.
		used := make(map[string]struct{}, len(node.members))
		for _, index := range order {
			base := transform(node.members[index].key)
			candidate := base
			for suffix := 2; ; suffix++ {
				if _, exists := used[candidate]; !exists {
					break
				}
				candidate = base + "~" + strconv.Itoa(suffix)
			}
			node.members[index].emitKey = candidate
			used[candidate] = struct{}{}
		}
	case orderedJSONArray:
		for _, item := range node.items {
			transformOrderedJSONValues(item, transform)
		}
	case orderedJSONString:
		node.text = transform(node.text)
	case orderedJSONNumber:
		transformOrderedJSONPrimitive(node, node.text, transform)
	case orderedJSONBool:
		transformOrderedJSONPrimitive(node, strconv.FormatBool(node.boolean), transform)
	case orderedJSONNull:
		transformOrderedJSONPrimitive(node, "null", transform)
	}
}

// transformOrderedJSONPrimitive preserves a primitive's exact token and type
// unless the transform changes it. Changed tokens become strings because a
// redaction marker is not necessarily valid JSON in the original type.
func transformOrderedJSONPrimitive(node *orderedJSONNode, source string, transform func(string) string) {
	transformed := transform(source)
	if transformed == source {
		return
	}
	node.kind = orderedJSONString
	node.text = transformed
}

// writeOrderedJSON emits compact JSON from the ordered tree. It deliberately
// writes stored number tokens directly instead of re-encoding them.
func writeOrderedJSON(out *bytes.Buffer, node *orderedJSONNode) error {
	switch node.kind {
	case orderedJSONObject:
		out.WriteByte('{')
		for index, member := range node.members {
			if index > 0 {
				out.WriteByte(',')
			}
			key, err := json.Marshal(member.emitKey)
			if err != nil {
				return fmt.Errorf("encode object key: %w", err)
			}
			out.Write(key)
			out.WriteByte(':')
			if err := writeOrderedJSON(out, member.value); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	case orderedJSONArray:
		out.WriteByte('[')
		for index, item := range node.items {
			if index > 0 {
				out.WriteByte(',')
			}
			if err := writeOrderedJSON(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case orderedJSONString:
		value, err := json.Marshal(node.text)
		if err != nil {
			return fmt.Errorf("encode string: %w", err)
		}
		out.Write(value)
	case orderedJSONNumber:
		out.WriteString(node.text)
	case orderedJSONBool:
		if node.boolean {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case orderedJSONNull:
		out.WriteString("null")
	default:
		return fmt.Errorf("unsupported JSON node kind %d", node.kind)
	}
	return nil
}
