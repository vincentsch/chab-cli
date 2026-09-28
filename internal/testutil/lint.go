package testutil

import (
	"testing"

	"github.com/vincentsch/chab-cli/internal/safetylint"
)

var (
	// Compatibility aliases keep existing test callers on the shared policy.
	TicketIDPattern   = safetylint.TicketIDPattern
	LocalPathPattern  = safetylint.LocalPathPattern
	SecretPairPattern = safetylint.SecretPairPattern
	URLPattern        = safetylint.URLPattern
)

// SafetyViolations returns one message per help/doc-safety rule broken by text:
// leaked ticket ids, local machine paths, key-shaped tokens, or non-allowlisted
// URL hosts or shapes.
func SafetyViolations(text string) []string {
	return safetylint.Violations(text)
}

// AssertSafe fails t for any help/doc-safety violation.
func AssertSafe(t *testing.T, label, text string) {
	t.Helper()
	for _, violation := range SafetyViolations(text) {
		t.Fatalf("%s %s in %q", label, violation, text)
	}
}
