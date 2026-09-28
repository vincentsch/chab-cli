package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/redact"
)

func TestRawValueHumanScalarUnquoted(t *testing.T) {
	tests := map[string]string{
		`"hello"`: "hello\n",
		`42`:      "42\n",
		`true`:    "true\n",
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			var buf bytes.Buffer
			output.RawValueHuman(&buf, json.RawMessage(raw))
			if buf.String() != want {
				t.Fatalf("RawValueHuman() = %q, want %q", buf.String(), want)
			}
		})
	}
}

func TestRawValueStructuredAndNullAsStableJSON(t *testing.T) {
	tests := []string{
		`{"b":2,"a":1}`,
		`[{"b":2,"a":1}]`,
		`null`,
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			var buf bytes.Buffer
			output.RawValueHuman(&buf, json.RawMessage(raw))
			want, err := output.StableJSONBytes(json.RawMessage(raw))
			if err != nil {
				t.Fatalf("StableJSONBytes() error = %v", err)
			}
			if buf.String() != string(want) {
				t.Fatalf("RawValueHuman() = %q, want %q", buf.String(), string(want))
			}
		})
	}
}

func TestRawValuePlainMirrorsHumanAndSanitizes(t *testing.T) {
	var data, prose bytes.Buffer
	output.RawValuePlain(&data, &prose, json.RawMessage(`"bad\u001b[31mname"`))
	if data.String() != "bad name\n" || prose.String() != "" {
		t.Fatalf("plain data=%q prose=%q", data.String(), prose.String())
	}
}

func TestRawValueStableJSONRedacts(t *testing.T) {
	var buf bytes.Buffer
	output.RawValueHuman(&buf, json.RawMessage(`{"api_key":"ak_test|secret"}`))
	if strings.Contains(buf.String(), "secret") {
		t.Fatalf("secret leaked: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "ak_test|[REDACTED]") {
		t.Fatalf("redaction missing: %s", buf.String())
	}
}

func TestRawValueComposesWithOrderedSemanticSanitation(t *testing.T) {
	registry := redact.NewRegistry()
	registry.RegisterSecret("opaque-credential")
	sanitized, err := redact.TransformOrderedJSON(
		[]byte(`{"opaque-credential":"opaque-credential","same":"opaque-credential","same":"kept","large":9007199254740993,"static":"ak_test|secret"}`),
		registry.RedactValue,
	)
	if err != nil {
		t.Fatalf("TransformOrderedJSON() error = %v", err)
	}

	var human bytes.Buffer
	output.RawValueHuman(&human, json.RawMessage(sanitized))
	var plainData, plainProse bytes.Buffer
	output.RawValuePlain(&plainData, &plainProse, json.RawMessage(sanitized))
	if plainProse.Len() != 0 {
		t.Fatalf("sanitized plain output wrote prose: %q", plainProse.String())
	}
	stable, err := output.StableJSONBytes(json.RawMessage(sanitized))
	if err != nil {
		t.Fatalf("StableJSONBytes() error = %v", err)
	}

	for name, text := range map[string]string{
		"human":       human.String(),
		"plain data":  plainData.String(),
		"stable JSON": string(stable),
	} {
		if strings.Contains(text, "opaque-credential") || strings.Contains(text, "secret") {
			t.Fatalf("%s leaked protected data: %s", name, text)
		}
		for _, want := range []string{
			`"[REDACTED]": "[REDACTED]"`,
			`"same": "[REDACTED]"`,
			`"same~2": "kept"`,
			`"large": 9007199254740993`,
			`"static": "ak_test|[REDACTED]"`,
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s missing %q:\n%s", name, want, text)
			}
		}
	}
}
