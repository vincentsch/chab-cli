package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/vincentsch/chab-cli/internal/redact"
)

// Error is a decoded API error envelope.
type Error struct {
	Code       string
	Message    string
	UserAction string
	Retryable  bool
	Details    map[string][]string
	// DetailsValue preserves Chab's full structured error.details value after
	// request-local redaction. It may be a scalar, array, object, nil, or an
	// additive future shape.
	DetailsValue any
	// DetailsPresent distinguishes an explicitly supplied JSON null details
	// value from an omitted details field.
	DetailsPresent bool
	// RawDetails preserves bootstrap-only error.details bytes that cannot fit
	// the validation-details shape, such as scalar slow_down interval hints.
	RawDetails json.RawMessage
	RequestID  string
	Status     int
	Meta       ResponseMeta
	// classifiedExitCode keeps the classification derived from the original
	// server code without retaining that potentially sensitive string. Decoded
	// codes may be redacted before the error reaches callers.
	classifiedExitCode int
}

// ProtocolError is an API contract or response-shape failure.
type ProtocolError struct {
	Detail    string
	Status    int
	RequestID string
	Err       error
	Meta      ResponseMeta
}

// TransportError is a network or response-body read failure. It deliberately
// carries no request id, so network failures cannot fabricate one.
type TransportError struct {
	Err        error
	Attempts   int
	RetryWaits []time.Duration
}

// UsageError is a local preflight failure before a usable API request is sent.
type UsageError struct {
	Field  string
	Detail string
	Err    error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("api error ")
	b.WriteString(e.Code)
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.UserAction != "" {
		b.WriteString(". Next step: ")
		b.WriteString(e.UserAction)
	}
	if e.Status != 0 {
		b.WriteString(fmt.Sprintf(" (status %d", e.Status))
		if e.RequestID != "" {
			b.WriteString(", request id ")
			b.WriteString(e.RequestID)
		}
		b.WriteString(")")
	} else if e.RequestID != "" {
		b.WriteString(" (request id ")
		b.WriteString(e.RequestID)
		b.WriteString(")")
	}
	return redact.String(b.String())
}

// ExitCode returns the classification captured while decoding. Errors built by
// other callers fall back to classifying their public code directly.
func (e *Error) ExitCode() int {
	if e == nil {
		return 2
	}
	if e.classifiedExitCode != 0 {
		return e.classifiedExitCode
	}
	if code, ok := exitCodes[e.Code]; ok {
		return code
	}
	return 2
}

func (e *ProtocolError) Error() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("api protocol error")
	if e.Detail != "" {
		b.WriteString(": ")
		b.WriteString(e.Detail)
	}
	if e.Status != 0 {
		b.WriteString(fmt.Sprintf(" (status %d", e.Status))
		if e.RequestID != "" {
			b.WriteString(", request id ")
			b.WriteString(e.RequestID)
		}
		b.WriteString(")")
	} else if e.RequestID != "" {
		b.WriteString(" (request id ")
		b.WriteString(e.RequestID)
		b.WriteString(")")
	}
	return redact.String(b.String())
}

func (e *ProtocolError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *ProtocolError) ExitCode() int {
	return 2
}

func (e *TransportError) Error() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("api network error after %d attempt(s)", e.Attempts))
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return redact.String(b.String())
}

func (e *TransportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *TransportError) ExitCode() int {
	return 2
}

func (e *UsageError) Error() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("api usage error")
	if e.Field != "" {
		b.WriteString(" for ")
		b.WriteString(e.Field)
	}
	if e.Detail != "" {
		b.WriteString(": ")
		b.WriteString(e.Detail)
	}
	return redact.String(b.String())
}

func (e *UsageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *UsageError) ExitCode() int {
	return 1
}

// exitCodes mirrors the table in output-and-errors.md.
var exitCodes = map[string]int{
	"api_token_missing":                  3,
	"invalid_api_token":                  3,
	"api_token_expired":                  3,
	"api_token_revoked":                  3,
	"guest_credential_expired":           3,
	"guest_credential_revoked":           3,
	"guest_credential_limit_exceeded":    6,
	"signup_required":                    4,
	"free_credits_exhausted":             4,
	"email_verification_required":        4,
	"upgrade_required":                   4,
	"free_usage_paused":                  6,
	"challenge_required":                 4,
	"market_not_eligible":                4,
	"operation_not_available_on_free":    4,
	"promotional_budget_exhausted":       6,
	"team_token_required":                3,
	"access_denied":                      3,
	"expired_token":                      3,
	"api_access_not_included":            4,
	"api_scope_missing":                  4,
	"api_token_disabled":                 4,
	"api_token_ip_not_allowed":           4,
	"operation_family_not_included":      4,
	"paid_plan_required":                 4,
	"subscription_unhealthy":             4,
	"not_found":                          5,
	"rate_limited":                       6,
	"temporarily_unavailable":            6,
	"provider_unavailable":               6,
	"provider_temporarily_rate_limited":  6,
	"provider_response_invalid":          6,
	"operation_family_degraded":          6,
	"connection_temporarily_unavailable": 6,
	"concurrency_limit_exceeded":         6,
	"idempotency_request_in_progress":    6,
	"validation_failed":                  1,
	"invalid_grant":                      2,
	"invalid_client":                     2,
	"invalid_request":                    1,
	"invalid_scope":                      1,
	"unsupported_grant_type":             2,
	"authorization_pending":              2,
	"slow_down":                          6,
	"server_error":                       2,
	"method_not_allowed":                 2,
	"idempotency_key_conflict":           2,
	"idempotency_key_required":           1,
	"idempotency_key_invalid":            1,
	"management_approval_required":       4,
	"management_approval_invalid":        4,
	"management_approval_expired":        4,
	"token_policy_version_conflict":      2,
	"max_budget_exceeded":                4,
	"insufficient_credits":               4,
	"api_token_spending_not_allowed":     4,
	"api_token_spending_budget_exceeded": 4,
	"token_spending_disabled":            4,
	"token_budget_exceeded":              4,
	"spend_threshold_exceeded":           4,
	"quota_exceeded":                     4,
	"operation_family_disabled":          4,
}
