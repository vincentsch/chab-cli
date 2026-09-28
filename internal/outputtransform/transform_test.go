package outputtransform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/outputtransform"
)

func TestExecuteJQ(t *testing.T) {
	stable := stableJSON(t, map[string]any{
		"version": "dev",
		"os":      "linux",
		"arch":    "amd64",
		"number":  float64(1),
		"large":   int64(9007199254740993),
		"huge":    json.RawMessage("184467440737095516160"),
		"html":    "a&b",
		"escape":  "quote\"slash\\control\x01雪<& ak_escape|escape-secret tail",
		"encoded": "encoded ak_encoded%7Cencoded-secret tail",
		"api_key": "ak_test|super-secret",
	})

	tests := []struct {
		name string
		expr string
		want string
	}{
		{name: "string", expr: ".version", want: "\"dev\"\n"},
		{name: "multiple", expr: ".os, .arch", want: "\"linux\"\n\"amd64\"\n"},
		{name: "object compact", expr: "{version, os}", want: "{\"os\":\"linux\",\"version\":\"dev\"}\n"},
		{name: "array compact", expr: "[.os, .arch]", want: "[\"linux\",\"amd64\"]\n"},
		{name: "empty", expr: "empty", want: ""},
		{name: "empty expression is identity", expr: "", want: "{\"api_key\":\"ak_test|[REDACTED]\",\"arch\":\"amd64\",\"encoded\":\"encoded ak_encoded%7C[REDACTED] tail\",\"escape\":\"quote\\\"slash\\\\control\\u0001雪<& ak_escape|[REDACTED] tail\",\"html\":\"a&b\",\"huge\":184467440737095516160,\"large\":9007199254740993,\"number\":1,\"os\":\"linux\",\"version\":\"dev\"}\n"},
		{name: "number", expr: ".number", want: "1\n"},
		{name: "large integer exact", expr: ".large", want: "9007199254740993\n"},
		{name: "integer beyond int64 exact", expr: ".huge", want: "184467440737095516160\n"},
		{name: "ampersand", expr: ".html", want: "\"a&b\"\n"},
		{name: "escape-heavy string", expr: ".escape", want: "\"quote\\\"slash\\\\control\\u0001雪<& ak_escape|[REDACTED] tail\"\n"},
		{name: "encoded secret string", expr: ".encoded", want: "\"encoded ak_encoded%7C[REDACTED] tail\"\n"},
		{name: "redacted input", expr: ".api_key", want: "\"ak_test|[REDACTED]\"\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := outputtransform.ExecuteJQ(context.Background(), stable, tt.expr)
			if err != nil {
				t.Fatalf("ExecuteJQ() error = %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("ExecuteJQ() = %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestExecuteJQErrorsAreClassifiedAndBuffered(t *testing.T) {
	stable := stableJSON(t, map[string]string{"version": "dev"})
	tests := []struct {
		name      string
		expr      string
		wantStage string
		wantText  string
	}{
		{name: "parse", expr: ".version |", wantStage: "parse", wantText: "invalid --jq expression:"},
		{name: "whitespace-only parse", expr: "   ", wantStage: "parse", wantText: "invalid --jq expression:"},
		{name: "compile", expr: "undefined_func", wantStage: "parse", wantText: "invalid --jq expression:"},
		{name: "runtime buffered", expr: `.version, error("boom")`, wantStage: "run", wantText: "--jq expression failed:"},
		{name: "halt error", expr: `halt_error(7)`, wantStage: "run", wantText: "--jq expression failed:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := outputtransform.ExecuteJQ(context.Background(), stable, tt.expr)
			if err == nil {
				t.Fatalf("ExecuteJQ() error = nil")
			}
			if len(got) != 0 {
				t.Fatalf("partial output = %q, want empty", string(got))
			}
			var transformErr *outputtransform.TransformError
			if !errors.As(err, &transformErr) {
				t.Fatalf("error type = %T, want TransformError", err)
			}
			if transformErr.Mode != "jq" || transformErr.Stage != tt.wantStage {
				t.Fatalf("mode/stage = %s/%s, want jq/%s", transformErr.Mode, transformErr.Stage, tt.wantStage)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantText)
			}
			if transformErr.ExitCode() != 1 {
				t.Fatalf("ExitCode() = %d, want 1", transformErr.ExitCode())
			}
		})
	}
}

func TestExecuteJQStructurallyRedactsSynthesizedValues(t *testing.T) {
	literalSecret := "ak_combo|combined-secret"
	encodedSecret := "ak_encoded%7Cencoded-secret"
	stable := stableJSON(t, map[string]any{
		"literal_prefix": "quote\"slash\\control\x01雪<& ak_combo|",
		"literal_suffix": "combined-secret tail",
		"encoded_prefix": "encoded ak_encoded%7C",
		"encoded_suffix": "encoded-secret end",
		"large":          json.RawMessage("184467440737095516160"),
	})
	got, err := outputtransform.ExecuteJQ(context.Background(), stable, `{joined: (.literal_prefix + .literal_suffix), encoded: (.encoded_prefix + .encoded_suffix), large, "ak_same|z": 1, "ak_same|a": 2}`)
	if err != nil {
		t.Fatalf("ExecuteJQ() error = %v", err)
	}
	for _, secret := range []string{literalSecret, encodedSecret, "combined-secret", "encoded-secret", "ak_same|z", "ak_same|a"} {
		if bytes.Contains(got, []byte(secret)) {
			t.Fatalf("jq secret %q survived: %s", secret, got)
		}
	}
	if bytes.Contains(got, []byte{0x01}) {
		t.Fatalf("jq secret survived: %s", got)
	}
	dec := json.NewDecoder(bytes.NewReader(got))
	dec.UseNumber()
	var decoded map[string]any
	if err := dec.Decode(&decoded); err != nil {
		t.Fatalf("jq output is not JSON: %v\n%s", err, got)
	}
	base := "ak_same|[REDACTED]"
	if decoded[base] != json.Number("2") || decoded[base+"~2"] != json.Number("1") || decoded["large"] != json.Number("184467440737095516160") {
		t.Fatalf("jq output changed values: %#v", decoded)
	}
	if joined, ok := decoded["joined"].(string); !ok || joined != "quote\"slash\\control\x01雪<& ak_combo|[REDACTED] tail" {
		t.Fatalf("joined value = %#v", decoded["joined"])
	}
	if encoded, ok := decoded["encoded"].(string); !ok || encoded != "encoded ak_encoded%7C[REDACTED] end" {
		t.Fatalf("encoded value = %#v", decoded["encoded"])
	}
}

func TestExecuteJQEscapesC1ControlsWithoutChangingDecodedValues(t *testing.T) {
	stable := stableJSON(t, map[string]string{
		"next\u0085line": "ready\u009b32mgreen",
	})
	got, err := outputtransform.ExecuteJQ(context.Background(), stable, ".")
	if err != nil {
		t.Fatalf("ExecuteJQ() error = %v", err)
	}
	if bytes.Contains(got, []byte("\u0085")) || bytes.Contains(got, []byte("\u009b")) {
		t.Fatalf("jq output contains raw C1 controls: %q", got)
	}
	if !bytes.Contains(got, []byte(`next\u0085line`)) || !bytes.Contains(got, []byte(`ready\u009b32mgreen`)) {
		t.Fatalf("jq output does not escape C1 controls: %q", got)
	}

	var decoded map[string]string
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("jq output is not parseable: %v\n%s", err, got)
	}
	if decoded["next\u0085line"] != "ready\u009b32mgreen" {
		t.Fatalf("decoded value changed: %#v", decoded)
	}
}

func TestExecuteJQEscapesSynthesizedC1Controls(t *testing.T) {
	stable := stableJSON(t, map[string]bool{"ok": true})
	got, err := outputtransform.ExecuteJQ(
		context.Background(),
		stable,
		`{status: ([155, 51, 49, 109, 114, 101, 100] | implode)}`,
	)
	if err != nil {
		t.Fatalf("ExecuteJQ() error = %v", err)
	}
	if bytes.Contains(got, []byte("\u009b")) || !bytes.Contains(got, []byte(`\u009b31mred`)) {
		t.Fatalf("jq output does not safely escape synthesized C1: %q", got)
	}

	var decoded map[string]string
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("jq output is not parseable: %v\n%s", err, got)
	}
	if decoded["status"] != "\u009b31mred" {
		t.Fatalf("decoded synthesized value changed: %#v", decoded)
	}
}

func FuzzExecuteJQIdentityPreservesStableStrings(f *testing.F) {
	var allC1 strings.Builder
	for r := rune('\u0080'); r <= '\u009f'; r++ {
		allC1.WriteRune(r)
	}
	for _, seed := range []string{
		"",
		"plain ASCII",
		"Größe 雪",
		allC1.String(),
		"\x00\x1b[31m\x7f",
		"ak_fuzz|fuzz-secret",
		"ak_encoded%7Cfuzz-secret",
		string([]byte{'a', 0xff, 0xfe, 'z'}),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value string) {
		stable := stableJSON(t, map[string]string{"value": value})
		var stableValue map[string]string
		if err := json.Unmarshal(stable, &stableValue); err != nil {
			t.Fatalf("stable JSON is not parseable: %v\n%s", err, stable)
		}

		got, err := outputtransform.ExecuteJQ(context.Background(), stable, ".value")
		if err != nil {
			t.Fatalf("ExecuteJQ() error = %v", err)
		}
		document := bytes.TrimSuffix(got, []byte{'\n'})
		for _, r := range string(document) {
			if r < '\u0020' || r == '\u007f' || (r >= '\u0080' && r <= '\u009f') {
				t.Fatalf("jq output contains raw terminal-control rune U+%04X: %q", r, got)
			}
		}

		var transformed string
		if err := json.Unmarshal(got, &transformed); err != nil {
			t.Fatalf("jq output is not one JSON string: %v\n%s", err, got)
		}
		if transformed != stableValue["value"] {
			t.Fatalf("jq identity changed stable value: before %q after %q", stableValue["value"], transformed)
		}
	})
}

func TestExecuteTemplate(t *testing.T) {
	stable := stableJSON(t, map[string]any{
		"version": "dev",
		"number":  float64(1),
		"large":   int64(9007199254740993),
		"huge":    json.RawMessage("184467440737095516160"),
		"api_key": "ak_test|super-secret",
		"unsafe":  "Bad\x1b[31mName",
		"escape":  "quote\"slash\\control\x01雪<& ak_escape|escape-secret tail",
		"encoded": "encoded ak_encoded%7Cencoded-secret tail",
	})
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "field", text: "{{.version}}", want: "dev\n"},
		{name: "keeps newline", text: "{{.version}}\n", want: "dev\n"},
		{name: "preserves multiple trailing newlines", text: "{{.version}}\n\n", want: "dev\n\n"},
		{name: "empty render", text: "", want: "\n"},
		{name: "number", text: "{{.number}}", want: "1\n"},
		{name: "large integer exact", text: "{{.large}}", want: "9007199254740993\n"},
		{name: "integer beyond int64 exact", text: "{{.huge}}", want: "184467440737095516160\n"},
		{name: "redacted input", text: "{{.api_key}}", want: "ak_test|[REDACTED]\n"},
		{name: "control bytes sanitized", text: "{{.unsafe}}", want: "Bad Name\n"},
		{name: "escape-heavy string", text: "{{.escape}}", want: "quote\"slash\\control 雪<& ak_escape|[REDACTED] tail\n"},
		{name: "encoded secret string", text: "{{.encoded}}", want: "encoded ak_encoded%7C[REDACTED] tail\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := outputtransform.ExecuteTemplate(stable, tt.text)
			if err != nil {
				t.Fatalf("ExecuteTemplate() error = %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("ExecuteTemplate() = %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestExecuteTemplateRedactsAndSanitizesSynthesizedValues(t *testing.T) {
	stable := stableJSON(t, map[string]string{
		"key_prefix":           "ak_combo",
		"key_separator":        "|",
		"key_suffix":           "combined-secret",
		"encoded_prefix":       "ak_encoded",
		"encoded_separator":    "%7C",
		"encoded_suffix":       "encoded-secret",
		"authorization_prefix": "Authorization: ",
		"authorization_scheme": "Bearer ",
		"authorization_secret": "bearer-secret",
		"escape_prefix":        "safe\x1b[",
		"escape_suffix":        "31mred",
		"utf8_control_prefix":  "left\u009b",
		"utf8_control_suffix":  "32mright",
	})
	got, err := outputtransform.ExecuteTemplate(
		stable,
		`{{.key_prefix}}{{.key_separator}}{{.key_suffix}} {{.encoded_prefix}}{{.encoded_separator}}{{.encoded_suffix}} {{.authorization_prefix}}{{.authorization_scheme}}{{.authorization_secret}} {{.escape_prefix}}{{.escape_suffix}} {{.utf8_control_prefix}}{{.utf8_control_suffix}}`,
	)
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}
	want := "ak_combo|[REDACTED] ak_encoded%7C[REDACTED] Authorization: Bearer [REDACTED] safe 31mred left 32mright\n"
	if string(got) != want {
		t.Fatalf("ExecuteTemplate() = %q, want %q", string(got), want)
	}
	for _, secret := range []string{"combined-secret", "encoded-secret", "bearer-secret"} {
		if bytes.Contains(got, []byte(secret)) {
			t.Fatalf("template output contains synthesized secret %q: %q", secret, got)
		}
	}
	if safe := output.SanitizeControlBytes(got); !bytes.Equal(safe, got) {
		t.Fatalf("template output contains terminal controls: %q", got)
	}
}

func TestExecuteTemplatePreservesOwnedDelimiterAfterProviderControl(t *testing.T) {
	stable := stableJSON(t, map[string]string{
		"name":   "provider\x1b",
		"status": "active",
	})
	got, err := outputtransform.ExecuteTemplate(stable, "{{.name}}|{{.status}}")
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}
	if want := "provider |active\n"; string(got) != want {
		t.Fatalf("ExecuteTemplate() = %q, want %q", string(got), want)
	}
}

func TestExecuteTemplateErrorsAreClassifiedAndBuffered(t *testing.T) {
	stable := stableJSON(t, map[string]string{"version": "dev"})
	tests := []struct {
		name      string
		text      string
		wantStage string
		wantText  string
	}{
		{name: "parse", text: "{{.bad", wantStage: "parse", wantText: "invalid --template:"},
		{name: "runtime buffered", text: "{{.version}}{{.missing}}", wantStage: "run", wantText: "--template rendering failed:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := outputtransform.ExecuteTemplate(stable, tt.text)
			if err == nil {
				t.Fatalf("ExecuteTemplate() error = nil")
			}
			if len(got) != 0 {
				t.Fatalf("partial output = %q, want empty", string(got))
			}
			var transformErr *outputtransform.TransformError
			if !errors.As(err, &transformErr) {
				t.Fatalf("error type = %T, want TransformError", err)
			}
			if transformErr.Mode != "template" || transformErr.Stage != tt.wantStage {
				t.Fatalf("mode/stage = %s/%s, want template/%s", transformErr.Mode, transformErr.Stage, tt.wantStage)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantText)
			}
		})
	}
}

func TestTransformValidation(t *testing.T) {
	if err := outputtransform.ValidateJQ(".version"); err != nil {
		t.Fatalf("ValidateJQ() error = %v", err)
	}
	if err := outputtransform.ValidateJQ(""); err != nil {
		t.Fatalf("ValidateJQ(empty) error = %v", err)
	}
	if err := outputtransform.ValidateJQ("   "); err == nil || !strings.HasPrefix(err.Error(), "invalid --jq expression:") {
		t.Fatalf("ValidateJQ(whitespace) = %v", err)
	}
	if err := outputtransform.ValidateTemplate("{{.version}}"); err != nil {
		t.Fatalf("ValidateTemplate() error = %v", err)
	}
	if err := outputtransform.ValidateJQ(".version |"); err == nil || !strings.HasPrefix(err.Error(), "invalid --jq expression:") {
		t.Fatalf("ValidateJQ(parse) = %v", err)
	}
	if err := outputtransform.ValidateJQ("undefined_func"); err == nil || !strings.HasPrefix(err.Error(), "invalid --jq expression:") {
		t.Fatalf("ValidateJQ(compile) = %v", err)
	}
	if err := outputtransform.ValidateTemplate("{{.bad"); err == nil || !strings.Contains(err.Error(), "template: --template:1: unclosed action") {
		t.Fatalf("ValidateTemplate() = %v", err)
	}
}

func TestTransformRejectsTrailingJSONValue(t *testing.T) {
	stable := []byte(`{"version":"dev"} {"extra":true}`)
	if got, err := outputtransform.ExecuteJQ(context.Background(), stable, "."); err == nil || len(got) != 0 || !strings.Contains(err.Error(), "invalid JSON after top-level value") {
		t.Fatalf("ExecuteJQ trailing JSON = %q, %v", string(got), err)
	}
	if got, err := outputtransform.ExecuteTemplate(stable, "{{.version}}"); err == nil || len(got) != 0 || !strings.Contains(err.Error(), "invalid JSON after top-level value") {
		t.Fatalf("ExecuteTemplate trailing JSON = %q, %v", string(got), err)
	}
}

func stableJSON(t *testing.T, value any) []byte {
	t.Helper()
	stable, err := output.StableJSONBytes(value)
	if err != nil {
		t.Fatalf("StableJSONBytes() error = %v", err)
	}
	return stable
}
