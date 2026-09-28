package testutil

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// AssertSuccess requires a zero exit and empty stderr.
func AssertSuccess(t *testing.T, r Result) {
	t.Helper()
	if r.ExitCode != 0 || r.Stderr != "" || r.Err != nil {
		t.Fatalf("command result = %#v, want success", r)
	}
}

// AssertExitCode requires an exact exit code without constraining stdout or
// stderr, so report-producing nonzero exits can assert their payloads.
func AssertExitCode(t *testing.T, r Result, want int) {
	t.Helper()
	if r.ExitCode != want {
		t.Fatalf("exit code = %d, want %d; result=%#v", r.ExitCode, want, r)
	}
}

// ParseJSONObject parses raw as a JSON object and fails the test on any other
// shape.
func ParseJSONObject(t *testing.T, raw string) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		t.Fatalf("JSON object parse error = %v; raw=%s", err, raw)
	}
	if obj == nil {
		t.Fatalf("JSON value is not an object: %s", raw)
	}
	return obj
}

// ParseJSONArray parses raw as an array of JSON objects and fails the test on
// any other shape.
func ParseJSONArray(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var arr []map[string]any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		t.Fatalf("JSON array parse error = %v; raw=%s", err, raw)
	}
	if arr == nil {
		t.Fatalf("JSON value is not an array: %s", raw)
	}
	return arr
}

// ParseJSONError asserts stderr is exactly one newline-terminated compact JSON
// line with a non-null "error" object and returns that object.
func ParseJSONError(t *testing.T, raw string) map[string]any {
	t.Helper()
	if !strings.HasSuffix(raw, "\n") || strings.Count(raw, "\n") != 1 {
		t.Fatalf("error JSON is not one newline-terminated line: %q", raw)
	}
	if strings.Contains(raw, "\n\n") || strings.Contains(raw, "  ") {
		t.Fatalf("error JSON is not compact: %q", raw)
	}
	var envelope struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("error JSON parse error = %v; raw=%s", err, raw)
	}
	if envelope.Error == nil {
		t.Fatalf("error JSON missing non-null error object: %s", raw)
	}
	return envelope.Error
}

// AssertKeys requires the exact top-level key set.
func AssertKeys(t *testing.T, obj map[string]any, want []string) {
	t.Helper()
	var got []string
	for key := range obj {
		got = append(got, key)
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keys = %#v, want %#v in %#v", got, want, obj)
	}
}

// AssertNoSecret checks the full id|secret value and the post-| suffix.
func AssertNoSecret(t *testing.T, haystack, key string) {
	t.Helper()
	parts := strings.SplitN(key, "|", 2)
	if strings.Contains(haystack, key) || (len(parts) == 2 && strings.Contains(haystack, parts[1])) {
		values := []string{key}
		if len(parts) == 2 {
			values = append(values, parts[1])
		}
		t.Fatalf("secret leaked in %q", redactWithSecrets(haystack, values...))
	}
}

// AssertNoSecretValue checks a raw value plus its URL-escaped and JSON-escaped
// forms.
func AssertNoSecretValue(t *testing.T, haystack, value string) {
	t.Helper()
	for _, form := range secretValueForms(value) {
		if strings.Contains(haystack, form) {
			t.Fatalf("secret value leaked as [REDACTED] in %q", redactWithSecrets(haystack, value))
		}
	}
}

// AssertNoANSIControl rejects every control byte except newline and tab.
func AssertNoANSIControl(t *testing.T, text string) {
	t.Helper()
	for i := 0; i < len(text); i++ {
		b := text[i]
		if b == '\n' || b == '\t' {
			continue
		}
		if b < 0x20 || b == 0x7f {
			t.Fatalf("control byte 0x%02x leaked in %q", b, text)
		}
	}
}
