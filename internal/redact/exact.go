package redact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// Registry holds exact per-invocation secret values. It redacts raw, URL-
// escaped, double-escaped, JSON-escaped, and statically redacted forms without
// storing process-global state.
type Registry struct {
	mu       sync.RWMutex
	values   []string
	replacer *strings.Replacer
}

// NewRegistry creates an empty per-invocation exact-secret registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// RegisterSecret adds a non-empty value to subsequent text and JSON redaction.
func (r *Registry) RegisterSecret(value string) {
	if r == nil || value == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.values {
		if existing == value {
			return
		}
	}
	r.values = append(r.values, value)
	r.replacer = exactReplacer(r.values)
}

// RedactJSON removes registered values from JSON string values while preserving
// object keys, punctuation, and non-string primitive types. It accepts either
// one JSON document or the newline-delimited JSON stream produced by jq.
func (r *Registry) RedactJSON(data []byte) []byte {
	if data == nil {
		return nil
	}
	replacer := r.snapshot()
	if replacer == nil {
		return append([]byte(nil), data...)
	}
	redacted, err := redactJSONValues(data, replacer)
	if err != nil {
		// Stable JSON is validated before it reaches this registry. Fail closed
		// on an unexpected scanner failure rather than returning secret bytes.
		return []byte("\"[REDACTED]\"\n")
	}
	return redacted
}

// RedactText removes registered values from human, plain, transformed, debug,
// and error text.
func (r *Registry) RedactText(data []byte) []byte {
	return r.redact(data)
}

// RedactValue removes registered values from one semantic string value. Callers
// use this before rendering so structural labels remain distinguishable from
// secret-derived data that happens to contain the same bytes.
func (r *Registry) RedactValue(value string) string {
	replacer := r.snapshot()
	if replacer == nil {
		return value
	}
	return replacer.Replace(value)
}

func (r *Registry) redact(data []byte) []byte {
	if data == nil {
		return nil
	}
	text := string(data)
	replacer := r.snapshot()
	if replacer != nil {
		text = replacer.Replace(text)
	}
	return []byte(text)
}

// snapshot returns an immutable replacer. RegisterSecret installs a new one
// instead of mutating the previous instance, so readers can use it after the
// lock is released.
func (r *Registry) snapshot() *strings.Replacer {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.replacer
}

// redactJSONValues scans encoded JSON without rebuilding objects or numbers.
// Only decoded string values are replaced; object keys and all other tokens are
// copied byte for byte so schemas and primitive types cannot change.
func redactJSONValues(data []byte, replacer *strings.Replacer) ([]byte, error) {
	var out bytes.Buffer
	for index := 0; index < len(data); {
		switch data[index] {
		case '"':
			end, err := jsonStringEnd(data, index)
			if err != nil {
				return nil, err
			}
			if jsonStringIsObjectKey(data, end) {
				out.Write(data[index:end])
				index = end
				continue
			}
			var decoded string
			if err := json.Unmarshal(data[index:end], &decoded); err != nil {
				return nil, err
			}
			encoded, err := json.Marshal(replacer.Replace(decoded))
			if err != nil {
				return nil, err
			}
			out.Write(encoded)
			index = end
		case '{', '}', '[', ']', ',', ':', ' ', '\t', '\n', '\r':
			out.WriteByte(data[index])
			index++
		default:
			end := index
			for end < len(data) && !strings.ContainsRune("{}[],: \t\n\r", rune(data[end])) {
				end++
			}
			if end == index {
				return nil, fmt.Errorf("invalid JSON token")
			}
			// Numbers, booleans, and null retain their original representation
			// and type. A matching text form is not evidence that the value was
			// derived from a registered string secret.
			out.Write(data[index:end])
			index = end
		}
	}
	if err := validateJSONStream(out.Bytes()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// jsonStringIsObjectKey distinguishes a quoted key from a quoted value by the
// colon that must follow an object key, allowing whitespace in between.
func jsonStringIsObjectKey(data []byte, end int) bool {
	for index := end; index < len(data); index++ {
		switch data[index] {
		case ' ', '\t', '\n', '\r':
			continue
		case ':':
			return true
		default:
			return false
		}
	}
	return false
}

// validateJSONStream accepts one or more complete JSON documents. jq can emit
// several newline-delimited results, while stable JSON normally emits one.
func validateJSONStream(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	documents := 0
	for {
		var value json.RawMessage
		err := decoder.Decode(&value)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("redacted JSON is invalid: %w", err)
		}
		documents++
	}
	if documents == 0 {
		return fmt.Errorf("redacted JSON is empty")
	}
	return nil
}

// jsonStringEnd finds the closing quote while skipping escaped characters.
func jsonStringEnd(data []byte, start int) (int, error) {
	escaped := false
	for index := start + 1; index < len(data); index++ {
		if escaped {
			escaped = false
			continue
		}
		switch data[index] {
		case '\\':
			escaped = true
		case '"':
			return index + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated JSON string")
}

// ExactForms returns the representations of a value that can appear in URLs,
// JSON, or diagnostics. Longer forms are returned first so prefixes cannot
// leave a suffix behind during replacement.
func ExactForms(value string) []string {
	if value == "" {
		return nil
	}
	seen := make(map[string]struct{})
	forms := make([]string, 0, 16)
	add := func(form string) {
		if form == "" {
			return
		}
		if _, exists := seen[form]; exists {
			return
		}
		seen[form] = struct{}{}
		forms = append(forms, form)
	}
	addEncoded := func(form string) {
		add(form)
		add(strings.NewReplacer("%257C", "%257c", "%7C", "%7c").Replace(form))
	}
	addValue := func(raw string) {
		add(raw)
		query := url.QueryEscape(raw)
		path := url.PathEscape(raw)
		for _, encoded := range []string{query, path} {
			addEncoded(encoded)
			addEncoded(url.QueryEscape(encoded))
			addEncoded(url.PathEscape(encoded))
		}
		if encoded, err := json.Marshal(raw); err == nil && len(encoded) >= 2 {
			add(string(encoded[1 : len(encoded)-1]))
		}
	}
	addValue(value)
	if static := String(value); static != value {
		addValue(static)
	}
	sort.SliceStable(forms, func(i, j int) bool { return len(forms[i]) > len(forms[j]) })
	return forms
}

// exactReplacer combines every registered representation into one longest-
// first replacer so a shorter prefix cannot leave part of a secret behind.
func exactReplacer(values []string) *strings.Replacer {
	seen := make(map[string]struct{})
	forms := make([]string, 0, len(values)*16)
	for _, value := range values {
		for _, form := range ExactForms(value) {
			if _, exists := seen[form]; exists {
				continue
			}
			seen[form] = struct{}{}
			forms = append(forms, form)
		}
	}
	if len(forms) == 0 {
		return nil
	}
	sort.SliceStable(forms, func(i, j int) bool { return len(forms[i]) > len(forms[j]) })
	pairs := make([]string, 0, len(forms)*2)
	for _, form := range forms {
		pairs = append(pairs, form, token)
	}
	return strings.NewReplacer(pairs...)
}
