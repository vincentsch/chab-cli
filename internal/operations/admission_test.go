package operations

import (
	"net/http"
	"testing"

	"github.com/vincentsch/chab-cli/internal/api"
)

func TestDefinitiveAdmissionDenial(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
		marker string
		denied bool
	}{
		{"released_rate_limit", 429, "rate_limited", "released", true},
		{"settled_conflict", 409, "idempotency_request_in_progress", "settled", true},
		{"released_service_unavailable", 503, "promotional_budget_exhausted", "released", true},
		{"settled_service_unavailable", 503, "provider_unavailable", "settled", true},
		{"unmarked_service_unavailable", 503, "provider_unavailable", "", false},
		{"unmarked_rate_limit", 429, "rate_limited", "", false},
		{"unmarked_in_progress", 409, "idempotency_request_in_progress", "", false},
		{"unmarked_timeout", 408, "request_timeout", "", false},
		{"free_paused", 429, "free_usage_paused", "", true},
		{"validation", 422, "validation_failed", "", true},
		{"server_error", 500, "server_error", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := &api.Error{Status: tc.status, Code: tc.code, Meta: api.ResponseMeta{HTTPStatus: tc.status, IdempotencyOutcome: tc.marker}}
			if got := DefinitiveAdmissionDenial(err); got != tc.denied {
				t.Fatalf("DefinitiveAdmissionDenial(%d %s %s) = %v, want %v", tc.status, tc.code, tc.marker, got, tc.denied)
			}
		})
	}
	if DefinitiveAdmissionDenial(http.ErrAbortHandler) {
		t.Fatal("transport error cannot be a definitive admission denial")
	}
	if !DefinitiveAdmissionDenial(&api.ProtocolError{Status: 429, Meta: api.ResponseMeta{IdempotencyOutcome: "released"}}) {
		t.Fatal("malformed released denial must remain terminal")
	}
	if DefinitiveAdmissionDenial(&api.ProtocolError{Status: 429}) {
		t.Fatal("unmarked malformed admission response is uncertain")
	}
	if !DefinitiveAdmissionDenial(&api.ProtocolError{Status: 503, Meta: api.ResponseMeta{IdempotencyOutcome: "released"}}) {
		t.Fatal("malformed released 503 denial must remain terminal")
	}
	if DefinitiveAdmissionDenial(&api.ProtocolError{Status: 503}) {
		t.Fatal("unmarked malformed 503 response is uncertain")
	}
}

func TestDefinitiveAdmissionDenialForSubmission(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		code         string
		marker       string
		priorUnknown bool
		denied       bool
	}{
		{"first_unmarked_scope", 403, "api_scope_missing", "", false, true},
		{"replay_unmarked_scope", 403, "api_scope_missing", "", true, false},
		{"replay_unmarked_conflict", 409, "idempotency_key_conflict", "", true, false},
		{"replay_unmarked_rate_limit", 429, "rate_limited", "", true, false},
		{"replay_settled_conflict", 409, "idempotency_key_conflict", "settled", true, false},
		{"replay_settled_service_unavailable", 503, "provider_unavailable", "settled", true, false},
		{"replay_released_service_unavailable", 503, "provider_unavailable", "released", true, true},
		{"first_settled_service_unavailable", 503, "provider_unavailable", "settled", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := &api.Error{Status: tc.status, Code: tc.code, Meta: api.ResponseMeta{IdempotencyOutcome: tc.marker}}
			if got := DefinitiveAdmissionDenialForSubmission(err, tc.priorUnknown); got != tc.denied {
				t.Fatalf("denied = %v, want %v", got, tc.denied)
			}
		})
	}
	for _, marker := range []string{"", "settled", "released"} {
		err := &api.ProtocolError{Status: 503, Meta: api.ResponseMeta{IdempotencyOutcome: marker}}
		if got := DefinitiveAdmissionDenialForSubmission(err, true); got != (marker == "released") {
			t.Fatalf("protocol error marker %q denied = %v", marker, got)
		}
	}
}
