package safetylint_test

import (
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/safetylint"
)

func TestViolationsAllowsDocumentedShapes(t *testing.T) {
	tests := []string{
		"https://github.com/vincentsch/chab-saas",
		"https://github.com/vincentsch/chab-saas/blob/main/framework/agent-access.md",
		"https://github.com/vincentsch/chab-saas/blob/main/framework/agent-access.md#hosted-mcp-planned",
		"https://github.com/vincentsch/chab-cli/releases",
		"https://github.com/vincentsch/chab-cli/releases/tag/v0.7.0",
		"https://github.com/vincentsch/chab-cli/releases/download/v0.7.0/chab_0.7.0_linux_amd64.tar.gz",
		"See https://github.com/vincentsch/chab-cli/releases.",
		"http://localhost:8080/readyz",
		`--base-url string (default "http://localhost")`,
		`--api-base-url string (default "http://localhost/v1")`,
		"https://example.test/api",
		"https://api.example.test/path",
		"https://www.chab.ai",
		"https://www.chab.ai/v1",
		"https://www.chab.ai/mcp",
		"`https://www.chab.ai/v1`",
	}

	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			if got := safetylint.Violations(text); len(got) != 0 {
				t.Fatalf("Violations(%q) = %#v, want none", text, got)
			}
		})
	}
}

func TestViolationsRejectsUnsafeShapes(t *testing.T) {
	keyShaped := strings.Join([]string{"sample", "secret"}, "|")
	tests := []struct {
		text string
		want string
	}{
		{text: "p12-345", want: "ticket id"},
		{text: "/home/person/repo", want: "local machine path"},
		{text: keyShaped, want: "key-shaped token"},
		{text: "https://github.com/other/repo/releases", want: "disallowed URL"},
		{text: "http://github.com/vincentsch/chab-saas", want: "disallowed URL"},
		{text: "https://github.com/vincentsch/chab-saas/issues", want: "disallowed URL"},
		{text: "https://github.com/vincentsch/chab-saas/blob/main/framework/other.md", want: "disallowed URL"},
		{text: "https://github.com/vincentsch/chab-saas/blob/main/framework/agent-access.md?token=example", want: "disallowed URL"},
		{text: "https://github.com/vincentsch/chab-saas/blob/main/framework/agent-access.md#other", want: "disallowed URL"},
		{text: "https://github.com/vincentsch/chab-saas/blob/main/framework/agent-access.md/extra", want: "disallowed URL"},
		{text: "https://github.com.evil.test/vincentsch/chab-saas", want: "disallowed URL"},
		{text: "https://user@github.com/vincentsch/chab-saas", want: "disallowed URL"},
		{text: "https://github.com/vincentsch/chab-cli/issues", want: "disallowed URL"},
		{text: "https://github.com/vincentsch/chab-cli/blob/main/x", want: "disallowed URL"},
		{text: "https://github.com/vincentsch/chab-cli/releases/latest", want: "disallowed URL"},
		{text: "https://github.com/vincentsch/chab-cli/releases/tag/v0.7.0/extra", want: "disallowed URL"},
		{text: "https://github.com/vincentsch/chab-cli/releases/download/v0.7.0/asset/extra", want: "disallowed URL"},
		{text: "https://evil.test/x", want: "disallowed URL"},
		{text: "https://www.chab.ai/private", want: "disallowed URL"},
		{text: "https://www.chab.ai/v1?key=secret", want: "disallowed URL"},
		{text: "https://www.chab.ai.evil.test/v1", want: "disallowed URL"},
		{text: "http://www.chab.ai", want: "disallowed URL"},
		{text: `--base-url string (default "https://evil.test")`, want: "disallowed URL"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := strings.Join(safetylint.Violations(tt.text), "\n")
			if !strings.Contains(got, tt.want) {
				t.Fatalf("Violations(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}
