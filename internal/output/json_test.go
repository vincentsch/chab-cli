package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/output"
)

func TestWriteJSONStableBytesAndEscaping(t *testing.T) {
	value := struct {
		Name string `json:"name"`
		HTML string `json:"html"`
	}{
		Name: "Demo",
		HTML: "<tag>&value>",
	}
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, value); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	wantBytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	wantBytes = append(wantBytes, '\n')
	if !bytes.Equal(buf.Bytes(), wantBytes) {
		t.Fatalf("WriteJSON() = %q, want %q", buf.String(), string(wantBytes))
	}
	stable, err := output.StableJSONBytes(value)
	if err != nil {
		t.Fatalf("StableJSONBytes() error = %v", err)
	}
	if !bytes.Equal(stable, buf.Bytes()) {
		t.Fatalf("StableJSONBytes() = %q, want WriteJSON bytes %q", string(stable), buf.String())
	}
	if !strings.Contains(buf.String(), `\u003c`) || !strings.HasSuffix(buf.String(), "\n") {
		t.Fatalf("JSON escaping/newline not pinned: %q", buf.String())
	}
}

func TestStableJSONEscapesDELAndC1ControlsWithoutChangingDecodedValues(t *testing.T) {
	value := map[string]string{
		"next\u007f\u0085line": "ready\u007f\u009b32mgreen",
	}

	got, err := output.StableJSONBytes(value)
	if err != nil {
		t.Fatalf("StableJSONBytes() error = %v", err)
	}
	if bytes.Contains(got, []byte("\u007f")) || bytes.Contains(got, []byte("\u0085")) || bytes.Contains(got, []byte("\u009b")) {
		t.Fatalf("stable JSON contains raw DEL or C1 controls: %q", got)
	}
	if !bytes.Contains(got, []byte(`next\u007f\u0085line`)) || !bytes.Contains(got, []byte(`ready\u007f\u009b32mgreen`)) {
		t.Fatalf("stable JSON does not escape DEL and C1 controls: %q", got)
	}

	var decoded map[string]string
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("stable JSON is not parseable: %v\n%s", err, got)
	}
	if decoded["next\u007f\u0085line"] != "ready\u007f\u009b32mgreen" {
		t.Fatalf("decoded value changed: %#v", decoded)
	}
}

func FuzzJSONC1EscapingPreservesDecodedStrings(f *testing.F) {
	var allC1 strings.Builder
	for r := rune('\u0080'); r <= '\u009f'; r++ {
		allC1.WriteRune(r)
	}
	for _, seed := range []string{
		"",
		"plain ASCII",
		"Größe 雪",
		"\u0080\u0085\u009b\u009c\u009d\u009f",
		allC1.String(),
		"\x00\x1b[31m\x7f",
		string([]byte{'a', 0xff, 0xfe, 'z'}),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value string) {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("json.Marshal(%q) error = %v", value, err)
		}
		original := append([]byte(nil), encoded...)
		escaped := output.EscapeJSONC1Controls(encoded)

		if !bytes.Equal(encoded, original) {
			t.Fatalf("EscapeJSONC1Controls mutated its input: before %q after %q", original, encoded)
		}
		for _, r := range string(escaped) {
			if r < '\u0020' || r == '\u007f' || (r >= '\u0080' && r <= '\u009f') {
				t.Fatalf("escaped JSON contains raw terminal-control rune U+%04X: %q", r, escaped)
			}
		}

		var before, after string
		if err := json.Unmarshal(original, &before); err != nil {
			t.Fatalf("json.Unmarshal(original) error = %v: %q", err, original)
		}
		if err := json.Unmarshal(escaped, &after); err != nil {
			t.Fatalf("json.Unmarshal(escaped) error = %v: %q", err, escaped)
		}
		if after != before {
			t.Fatalf("decoded string changed: before %q after %q", before, after)
		}
	})
}

func TestWriteJSONMarshalFailure(t *testing.T) {
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, make(chan int)); err == nil {
		t.Fatalf("WriteJSON() error = nil, want marshal error")
	}
	if buf.String() != "" {
		t.Fatalf("output on marshal failure = %q, want empty", buf.String())
	}
}

func TestWriteJSONRedactsAndRemainsParseable(t *testing.T) {
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, map[string]string{"api_key": "ak_test|super-secret"}); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	if strings.Contains(buf.String(), "super-secret") {
		t.Fatalf("secret leaked: %s", buf.String())
	}
	var got map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("redacted output is not JSON: %v\n%s", err, buf.String())
	}
	if got["api_key"] != "ak_test|[REDACTED]" {
		t.Fatalf("api_key = %q", got["api_key"])
	}
}

func TestWriteJSONRedactsAuthorizationBearerAndRemainsParseable(t *testing.T) {
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, map[string]string{"note": "Authorization: Bearer super-secret"}); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	if strings.Contains(buf.String(), "super-secret") {
		t.Fatalf("secret leaked: %s", buf.String())
	}
	var got map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("redacted output is not JSON: %v\n%s", err, buf.String())
	}
	if got["note"] != "Authorization: Bearer [REDACTED]" {
		t.Fatalf("note = %q", got["note"])
	}
}

func TestStableJSONStructuredRedactionPreservesDocumentShape(t *testing.T) {
	rawSecret := "ak_escape|escape-secret"
	escapeHeavy := "quote\"slash\\control\x01雪<& " + rawSecret + " tail"
	value := struct {
		First string          `json:"first"`
		Data  json.RawMessage `json:"data"`
		Last  string          `json:"last"`
	}{
		First: escapeHeavy,
		Data:  json.RawMessage(`{"outer":{"ak_same|z":1,"ak_same|a":2,"plain":"quote\"slash\\control\u0001雪<& ak_encoded%7Cencoded-secret tail"},"large":184467440737095516160}`),
		Last:  "unchanged",
	}

	got, err := output.StableJSONBytes(value)
	if err != nil {
		t.Fatalf("StableJSONBytes() error = %v", err)
	}
	for _, secret := range []string{rawSecret, "escape-secret", "ak_same|z", "ak_same|a", "ak_encoded%7Cencoded-secret", "encoded-secret"} {
		if strings.Contains(string(got), secret) {
			t.Fatalf("redacted JSON contains %q: %s", secret, got)
		}
	}
	if !bytes.HasPrefix(got, []byte("{\n  \"first\":")) ||
		!bytes.Contains(got, []byte("\n  \"data\":")) ||
		!bytes.HasSuffix(got, []byte("\n  \"last\": \"unchanged\"\n}\n")) {
		t.Fatalf("struct field order changed: %s", got)
	}
	if !bytes.Contains(got, []byte("184467440737095516160")) {
		t.Fatalf("large integer changed: %s", got)
	}

	var decoded struct {
		First string `json:"first"`
		Data  struct {
			Outer map[string]any `json:"outer"`
			Large json.Number    `json:"large"`
		} `json:"data"`
		Last string `json:"last"`
	}
	dec := json.NewDecoder(bytes.NewReader(got))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		t.Fatalf("redacted output is not JSON: %v\n%s", err, got)
	}
	base := "ak_same|[REDACTED]"
	if decoded.Data.Outer[base] != json.Number("2") || decoded.Data.Outer[base+"~2"] != json.Number("1") || decoded.Data.Outer["plain"] != "quote\"slash\\control\x01雪<& ak_encoded%7C[REDACTED] tail" {
		t.Fatalf("redacted collision object = %#v", decoded.Data.Outer)
	}
	if decoded.First != "quote\"slash\\control\x01雪<& ak_escape|[REDACTED] tail" || decoded.Data.Large.String() != "184467440737095516160" || decoded.Last != "unchanged" {
		t.Fatalf("decoded output changed: %#v", decoded)
	}
}
