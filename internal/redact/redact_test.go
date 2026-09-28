package redact_test

import (
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/redact"
)

func TestStringRedactsLiteralAndEncodedAPIKeys(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "literal text", in: "key ak_demo|secret-value next", want: "key ak_demo|[REDACTED] next"},
		{name: "uppercase encoded path", in: "/projects/ak_demo%7Csecret-value tail", want: "/projects/ak_demo%7C[REDACTED] tail"},
		{name: "lowercase encoded query", in: "key=ak_demo%7csecret-value next", want: "key=ak_demo%7c[REDACTED] next"},
		{name: "encoded value in query", in: "key=ak_demo%257Csecret-value next", want: "key=ak_demo%257C[REDACTED] next"},
		{name: "quoted delimiter", in: `"ak_demo%7Csecret-value"`, want: `"ak_demo%7C[REDACTED]"`},
		{name: "escape-heavy surroundings", in: "quote\" slash\\ control\x01雪<& ak_demo|secret-value tail", want: "quote\" slash\\ control\x01雪<& ak_demo|[REDACTED] tail"},
		{name: "id too short", in: "a|secret-value", want: "a|secret-value"},
		{name: "wrong escape", in: "ak_demo%7Bsecret-value", want: "ak_demo%7Bsecret-value"},
		{name: "separator without secret", in: "ak_demo%7C", want: "ak_demo%7C"},
		{name: "ordinary pipe-separated value", in: "project-1|active", want: "project-1|active"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redact.String(tt.in); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStringRedactionDoesNotNormalizeEncodedAPIKeys(t *testing.T) {
	for _, separator := range []string{"|", "%7C", "%7c", "%257C", "%257c"} {
		input := "prefix ak_demo" + separator + "secret-value suffix"
		got := redact.String(input)
		if strings.Contains(got, "secret-value") {
			t.Fatalf("String(%q) leaked secret: %q", input, got)
		}
		if !strings.Contains(got, "ak_demo"+separator+"[REDACTED]") {
			t.Fatalf("String(%q) changed separator: %q", input, got)
		}
	}
}
