package doctor

import (
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/api"
)

func TestSignupRequiredDoesNotAssumeGuestClaim(t *testing.T) {
	findings := apiErrorFinding(&api.Error{Code: "signup_required", Message: "Guest unavailable"})
	if len(findings) != 1 || findings[0].ID != "credential.source" {
		t.Fatalf("unexpected doctor findings: %#v", findings)
	}
	if findings[0].Summary != "guest trial cannot continue" {
		t.Fatalf("unexpected summary: %q", findings[0].Summary)
	}
	if !strings.Contains(findings[0].Detail, "expired or claimed") {
		t.Fatalf("missing eligible states: %q", findings[0].Detail)
	}
}
