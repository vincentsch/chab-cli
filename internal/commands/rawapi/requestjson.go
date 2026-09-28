package rawapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

type requestJSONKind uint8

const (
	requestJSONObject requestJSONKind = iota
	requestJSONArray
	requestJSONString
	requestJSONNumber
	requestJSONBool
	requestJSONNull
)

type requestJSONMember struct {
	key   string
	value *requestJSONNode
}

// requestJSONNode retains the original raw subtree alongside decoded ordered
// membership. The command uses it only for previews and diagnostic redaction;
// the validated original request bytes remain unchanged on the wire.
type requestJSONNode struct {
	kind    requestJSONKind
	raw     json.RawMessage
	members []requestJSONMember
	items   []*requestJSONNode
	text    string
	boolean bool
}

// requestJSONParser pairs token decoding with offsets into the validated source
// bytes. The decoded tree drives matching and previews, while each raw subtree
// retains the caller's original escape spelling and number text.
type requestJSONParser struct {
	raw     []byte
	decoder *json.Decoder
}

// parseRequestJSON parses exactly one JSON value and rejects trailing input.
// A token tree is used instead of maps so duplicate object members remain
// visible to both masking and secret collection.
func parseRequestJSON(raw json.RawMessage) (*requestJSONNode, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	parser := &requestJSONParser{raw: raw, decoder: decoder}
	node, err := parser.parseValue()
	if err != nil {
		return nil, fmt.Errorf("parse request JSON: %w", err)
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("parse request JSON: trailing input: %w", err)
		}
		return nil, fmt.Errorf("parse request JSON: trailing token %v", token)
	}
	return node, nil
}

// parseValue records the source byte range before decoding the semantic value.
// Compound values recurse so every child has its own exact raw subtree.
func (p *requestJSONParser) parseValue() (*requestJSONNode, error) {
	start, err := requestJSONValueStart(p.raw, p.decoder.InputOffset())
	if err != nil {
		return nil, err
	}
	token, err := p.decoder.Token()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("unexpected end of input")
		}
		return nil, fmt.Errorf("decode value: %w", err)
	}

	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			node := &requestJSONNode{kind: requestJSONObject}
			for p.decoder.More() {
				keyToken, err := p.decoder.Token()
				if err != nil {
					return nil, fmt.Errorf("decode object key: %w", err)
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("object key has unsupported token type %T", keyToken)
				}
				child, err := p.parseValue()
				if err != nil {
					return nil, err
				}
				node.members = append(node.members, requestJSONMember{key: key, value: child})
			}
			end, err := p.decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("decode object end: %w", err)
			}
			if end != json.Delim('}') {
				return nil, fmt.Errorf("object ended with unsupported token %v", end)
			}
			node.raw = requestJSONRawValue(p.raw, start, p.decoder.InputOffset())
			return node, nil
		case '[':
			node := &requestJSONNode{kind: requestJSONArray}
			for p.decoder.More() {
				child, err := p.parseValue()
				if err != nil {
					return nil, err
				}
				node.items = append(node.items, child)
			}
			end, err := p.decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("decode array end: %w", err)
			}
			if end != json.Delim(']') {
				return nil, fmt.Errorf("array ended with unsupported token %v", end)
			}
			node.raw = requestJSONRawValue(p.raw, start, p.decoder.InputOffset())
			return node, nil
		default:
			return nil, fmt.Errorf("unsupported delimiter %q", value)
		}
	case string:
		return &requestJSONNode{
			kind: requestJSONString,
			raw:  requestJSONRawValue(p.raw, start, p.decoder.InputOffset()),
			text: value,
		}, nil
	case json.Number:
		return &requestJSONNode{kind: requestJSONNumber, raw: requestJSONRawValue(p.raw, start, p.decoder.InputOffset())}, nil
	case bool:
		return &requestJSONNode{kind: requestJSONBool, raw: requestJSONRawValue(p.raw, start, p.decoder.InputOffset()), boolean: value}, nil
	case nil:
		return &requestJSONNode{kind: requestJSONNull, raw: requestJSONRawValue(p.raw, start, p.decoder.InputOffset())}, nil
	default:
		return nil, fmt.Errorf("unsupported token type %T", token)
	}
}

// requestJSONValueStart advances from the decoder's previous token boundary to
// the next value. At this point commas and colons can only be JSON separators;
// punctuation inside strings has already been consumed as part of a token.
func requestJSONValueStart(raw []byte, offset int64) (int, error) {
	for index := int(offset); index < len(raw); index++ {
		switch raw[index] {
		case ' ', '\t', '\n', '\r', ',', ':':
			continue
		default:
			return index, nil
		}
	}
	return 0, fmt.Errorf("unexpected end of input")
}

func requestJSONRawValue(raw []byte, start int, end int64) json.RawMessage {
	return append(json.RawMessage(nil), raw[start:int(end)]...)
}

// maskedRequestJSON creates the deterministic compact body shown by dry-run.
// Selected members keep their original positions but use a fixed mask.
func maskedRequestJSON(node *requestJSONNode, secret map[string]bool) (string, error) {
	var out bytes.Buffer
	if err := writeMaskedRequestJSON(&out, node, secret); err != nil {
		return "", err
	}
	return out.String(), nil
}

func writeMaskedRequestJSON(out *bytes.Buffer, node *requestJSONNode, secret map[string]bool) error {
	switch node.kind {
	case requestJSONObject:
		out.WriteByte('{')
		for index, member := range node.members {
			if index > 0 {
				out.WriteByte(',')
			}
			key, err := json.Marshal(member.key)
			if err != nil {
				return fmt.Errorf("encode request JSON key: %w", err)
			}
			out.Write(key)
			out.WriteByte(':')
			if secret[member.key] {
				out.WriteString(`"[REDACTED]"`)
				continue
			}
			if err := writeMaskedRequestJSON(out, member.value, secret); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	case requestJSONArray:
		out.WriteByte('[')
		for index, item := range node.items {
			if index > 0 {
				out.WriteByte(',')
			}
			if err := writeMaskedRequestJSON(out, item, secret); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case requestJSONString:
		value, err := json.Marshal(node.text)
		if err != nil {
			return fmt.Errorf("encode request JSON string: %w", err)
		}
		out.Write(value)
	case requestJSONNumber:
		out.Write(node.raw)
	case requestJSONBool:
		out.WriteString(strconv.FormatBool(node.boolean))
	case requestJSONNull:
		out.WriteString("null")
	default:
		return fmt.Errorf("unsupported request JSON node")
	}
	return nil
}

// collectRequestJSONSecrets visits every duplicate occurrence and matches
// decoded keys, so literal and Unicode-escaped spellings are treated alike.
func collectRequestJSONSecrets(node *requestJSONNode, secret map[string]bool, add func(string)) {
	switch node.kind {
	case requestJSONObject:
		for _, member := range node.members {
			if secret[member.key] {
				collectSelectedRequestJSONValue(member.value, add)
			}
			collectRequestJSONSecrets(member.value, secret, add)
		}
	case requestJSONArray:
		for _, item := range node.items {
			collectRequestJSONSecrets(item, secret, add)
		}
	}
}

// collectSelectedRequestJSONValue protects both forms that an API may echo. A
// compound selection contributes its compact original subtree and each decoded
// scalar below it; a scalar contributes its decoded semantic value.
func collectSelectedRequestJSONValue(node *requestJSONNode, add func(string)) {
	switch node.kind {
	case requestJSONString:
		add(node.text)
	case requestJSONNumber, requestJSONBool, requestJSONNull:
		add(string(node.raw))
	case requestJSONObject, requestJSONArray:
		var compact bytes.Buffer
		if err := json.Compact(&compact, node.raw); err == nil {
			add(compact.String())
		}
		collectRequestJSONScalarDescendants(node, add)
	}
}

// collectRequestJSONScalarDescendants walks through containers without adding
// partial container encodings; only leaf values are added by the base case.
func collectRequestJSONScalarDescendants(node *requestJSONNode, add func(string)) {
	switch node.kind {
	case requestJSONObject:
		for _, member := range node.members {
			collectRequestJSONScalarDescendants(member.value, add)
		}
	case requestJSONArray:
		for _, item := range node.items {
			collectRequestJSONScalarDescendants(item, add)
		}
	default:
		collectSelectedRequestJSONValue(node, add)
	}
}
