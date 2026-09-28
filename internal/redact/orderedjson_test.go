package redact_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/redact"
)

func TestTransformOrderedJSONPreservesShapeAndTransformsSemanticStrings(t *testing.T) {
	t.Parallel()

	input := []byte(`{"z":"secret","dup":"first","dup":{"nested":"secret"},"arr":["secret",9007199254740993,true,false,null],"escaped\u006bey":"secret"}`)
	got, err := redact.TransformOrderedJSON(input, func(value string) string {
		return strings.ReplaceAll(value, "secret", "[REDACTED]")
	})
	if err != nil {
		t.Fatalf("TransformOrderedJSON() error = %v", err)
	}
	want := `{"z":"[REDACTED]","dup":"first","dup~2":{"nested":"[REDACTED]"},"arr":["[REDACTED]",9007199254740993,true,false,null],"escapedkey":"[REDACTED]"}`
	if string(got) != want {
		t.Fatalf("TransformOrderedJSON() = %s, want %s", got, want)
	}
}

func TestTransformOrderedJSONHandlesTopLevelPrimitives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "string", input: `"opaque"`, want: `"[REDACTED]"`},
		{name: "number", input: `9007199254740993`, want: `9007199254740993`},
		{name: "true", input: `true`, want: `true`},
		{name: "false", input: `false`, want: `false`},
		{name: "null", input: `null`, want: `null`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := redact.TransformOrderedJSON([]byte(test.input), func(value string) string {
				if value == "opaque" {
					return "[REDACTED]"
				}
				return value
			})
			if err != nil {
				t.Fatalf("TransformOrderedJSON() error = %v", err)
			}
			if string(got) != test.want {
				t.Fatalf("TransformOrderedJSON() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestTransformOrderedJSONRedactsMatchingPrimitiveTokens(t *testing.T) {
	t.Parallel()

	input := []byte(`{"number":123456,"true":true,"false":false,"null":null,"safe":9007199254740993}`)
	got, err := redact.TransformOrderedJSON(input, func(value string) string {
		switch value {
		case "123456", "true", "null":
			return "[REDACTED]"
		default:
			return value
		}
	})
	if err != nil {
		t.Fatalf("TransformOrderedJSON() error = %v", err)
	}
	want := `{"number":"[REDACTED]","[REDACTED]~2":"[REDACTED]","false":false,"[REDACTED]":"[REDACTED]","safe":9007199254740993}`
	if string(got) != want {
		t.Fatalf("TransformOrderedJSON() = %s, want %s", got, want)
	}
}

func TestTransformOrderedJSONAllocatesCollidingKeysDeterministically(t *testing.T) {
	t.Parallel()

	input := []byte(`{"z":"z","a":"a","b":"b","a":"again"}`)
	transform := func(value string) string {
		switch value {
		case "a", "b", "z":
			return "same"
		default:
			return value
		}
	}
	first, err := redact.TransformOrderedJSON(input, transform)
	if err != nil {
		t.Fatalf("TransformOrderedJSON() error = %v", err)
	}
	second, err := redact.TransformOrderedJSON(input, transform)
	if err != nil {
		t.Fatalf("TransformOrderedJSON() repeat error = %v", err)
	}
	want := `{"same~4":"same","same":"same","same~3":"same","same~2":"again"}`
	if string(first) != want {
		t.Fatalf("TransformOrderedJSON() = %s, want %s", first, want)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("TransformOrderedJSON() is not deterministic: %s != %s", first, second)
	}
}

func TestTransformOrderedJSONFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		transform func(string) string
	}{
		{name: "nil transform", input: `{}`, transform: nil},
		{name: "empty", input: ``, transform: func(value string) string { return value }},
		{name: "malformed", input: `{"a":`, transform: func(value string) string { return value }},
		{name: "trailing", input: `{} []`, transform: func(value string) string { return value }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := redact.TransformOrderedJSON([]byte(test.input), test.transform)
			if err == nil {
				t.Fatal("TransformOrderedJSON() error = nil")
			}
			if !strings.Contains(err.Error(), "transform ordered JSON") {
				t.Fatalf("TransformOrderedJSON() error = %q, want wrapped context", err)
			}
			if got != nil {
				t.Fatalf("TransformOrderedJSON() output = %q, want nil", got)
			}
		})
	}
}
