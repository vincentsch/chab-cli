package output

import (
	"io"
)

// StableJSONBytes renders the command-owned machine value as stable JSON:
// two-space indentation, trailing newline, Go's default HTML escaping,
// terminal-safe C1 escaping, and redaction defense-in-depth.
func StableJSONBytes(value any) ([]byte, error) {
	out, err := structuredJSONBytes(value, true)
	if err != nil {
		return nil, err
	}
	out = append(out, '\n')
	return out, nil
}

// WriteJSON writes StableJSONBytes to w.
func WriteJSON(w io.Writer, value any) error {
	out, err := StableJSONBytes(value)
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}
