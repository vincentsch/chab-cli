package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type rawValueKind int

const (
	rawUnknown rawValueKind = iota
	rawObject
	rawArray
	rawString
	rawNumber
	rawBool
	rawNull
)

// RawValueHuman renders arbitrary decoded JSON for raw API commands. Scalars
// print unquoted; objects, arrays, and null print as stable JSON.
func RawValueHuman(w io.Writer, raw json.RawMessage) {
	writeRawValue(w, raw)
}

// RawValuePlain mirrors RawValueHuman onto the copy-safe data stream.
func RawValuePlain(data, _ io.Writer, raw json.RawMessage) {
	writeRawValue(data, raw)
}

// writeRawValue gives scalar raw responses a copy-friendly display while still
// sending structured values through the stable JSON writer. The command-level
// output boundary applies final redaction and ANSI/control-byte filtering.
func writeRawValue(w io.Writer, raw json.RawMessage) {
	switch rawValueKindOf(raw) {
	case rawString:
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			writeStableRawValue(w, raw)
			return
		}
		fmt.Fprintln(w, sanitizeControlString(value))
	case rawNumber, rawBool:
		fmt.Fprintln(w, sanitizeControlString(strings.TrimSpace(string(raw))))
	default:
		writeStableRawValue(w, raw)
	}
}

func writeStableRawValue(w io.Writer, raw json.RawMessage) {
	out, err := StableJSONBytes(raw)
	if err != nil {
		fmt.Fprintln(w, "null")
		return
	}
	_, _ = w.Write(out)
}

// rawValueKindOf classifies by the first non-space byte. That is enough to
// choose display behavior without decoding arbitrary raw API response shapes.
func rawValueKindOf(raw json.RawMessage) rawValueKind {
	for _, b := range raw {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{':
			return rawObject
		case '[':
			return rawArray
		case '"':
			return rawString
		case 't', 'f':
			return rawBool
		case 'n':
			return rawNull
		case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return rawNumber
		default:
			return rawUnknown
		}
	}
	return rawUnknown
}
