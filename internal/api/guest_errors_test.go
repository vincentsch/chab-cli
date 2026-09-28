package api

import "testing"

func TestGuestLifecycleExitCodes(t *testing.T) {
	for code, want := range map[string]int{
		"guest_credential_expired":        3,
		"guest_credential_revoked":        3,
		"guest_credential_limit_exceeded": 6,
		"signup_required":                 4,
		"free_credits_exhausted":          4,
		"free_usage_paused":               6,
		"challenge_required":              4,
		"market_not_eligible":             4,
		"operation_not_available_on_free": 4,
		"promotional_budget_exhausted":    6,
	} {
		if got := (&Error{Code: code}).ExitCode(); got != want {
			t.Errorf("%s exit code = %d, want %d", code, got, want)
		}
	}
}
