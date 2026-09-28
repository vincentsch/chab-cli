package testutil_test

import (
	"testing"

	"github.com/vincentsch/chab-cli/internal/safetylint"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestSafetyPolicyCompatibilityAliasesAndDelegate(t *testing.T) {
	if testutil.TicketIDPattern != safetylint.TicketIDPattern ||
		testutil.LocalPathPattern != safetylint.LocalPathPattern ||
		testutil.SecretPairPattern != safetylint.SecretPairPattern ||
		testutil.URLPattern != safetylint.URLPattern {
		t.Fatal("testutil safety pattern aliases do not reference the shared policy")
	}
	text := "https://not-allowed.invalid/path"
	got := testutil.SafetyViolations(text)
	want := safetylint.Violations(text)
	if len(got) != len(want) || len(got) == 0 || got[0] != want[0] {
		t.Fatalf("SafetyViolations(%q) = %#v, shared policy = %#v", text, got, want)
	}
}
