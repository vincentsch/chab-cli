package output

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/redact"
)

// APIErrorContext carries non-secret display context for API errors. It wraps
// the original error so the central exit-code and JSON-error paths still see
// the typed API error underneath.
type APIErrorContext struct {
	Err          error
	Profile      string
	KeyDisplayID string
}

// Only compact public token identifiers are safe to echo from the auth file.
// Arbitrary cached text must never become a Key line in an error.
var keyDisplayIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func (e *APIErrorContext) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// Unwrap exposes the original typed API error for errors.As callers.
func (e *APIErrorContext) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// WithCredentialContext wraps an API-client error with safe profile/key display
// metadata for the central human renderer.
func WithCredentialContext(err error, profile, keyDisplayID string) error {
	if err == nil {
		return nil
	}
	return &APIErrorContext{Err: err, Profile: profile, KeyDisplayID: keyDisplayID}
}

// WriteAPIErrorHuman renders a structured human API error and reports whether
// err was an API-client failure.
func WriteAPIErrorHuman(w io.Writer, err error) bool {
	var b strings.Builder
	switch {
	case writeHumanProtocolError(&b, err):
	case writeHumanAPIError(&b, err):
	case writeHumanTransportError(&b, err):
	default:
		return false
	}
	writeHumanAPIErrorNotes(&b, err)
	writeHumanAPIErrorContext(&b, err)

	// Scalar sanitation prevents line injection; this final pass also catches
	// any secret shape formed only after the complete message is assembled.
	_, writeErr := w.Write(redact.Bytes([]byte(b.String())))
	return writeErr == nil
}

// writeHumanAPIError renders a decoded API error through errors.As so an
// APIErrorContext wrapper cannot change the underlying type or exit code.
func writeHumanAPIError(b *strings.Builder, err error) bool {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	code := sanitizeErrorScalar(apiErr.Code)
	if code == "" {
		code = "api_error"
	}
	message := sanitizeErrorScalar(apiErr.Message)
	if message == "" {
		message = code
	}
	fmt.Fprintf(b, "Error: %s\n", message)
	fmt.Fprintf(b, "Code: %s\n", code)
	fmt.Fprintf(b, "Retryable: %t\n", apiErr.Retryable)
	if requestID := sanitizeErrorScalar(apiErr.RequestID); requestID != "" {
		fmt.Fprintf(b, "Request ID: %s\n", requestID)
	}
	if rows := validationDetailRows(apiErr.Details); len(rows) > 0 {
		writeDetailRows(b, rows)
	} else {
		writeDetailRows(b, rawDetailRows(structuredRawDetails(apiErr.RawDetails)))
	}
	if remediation := apiErrorRemediation(code); remediation != "" {
		fmt.Fprintf(b, "Remediation: %s\n", remediation)
	}
	writeHumanOperationalContext(b, apiErr.Meta)
	return true
}

// writeHumanProtocolError gives malformed API responses a stable local code
// while retaining a safe response request id when one was captured.
func writeHumanProtocolError(b *strings.Builder, err error) bool {
	var protocolErr *api.ProtocolError
	if !errors.As(err, &protocolErr) {
		return false
	}
	message := sanitizeErrorScalar(protocolErr.Detail)
	if message == "" {
		message = "api protocol error"
	}
	fmt.Fprintf(b, "Error: %s\n", message)
	b.WriteString("Code: api_protocol_error\n")
	if requestID := sanitizeErrorScalar(protocolErr.RequestID); requestID != "" {
		fmt.Fprintf(b, "Request ID: %s\n", requestID)
	}
	writeHumanOperationalContext(b, protocolErr.Meta)
	return true
}

// writeHumanOperationalContext appends response-backed retry and rate-limit
// details after the main error body, preserving headers that were present but
// empty.
func writeHumanOperationalContext(b *strings.Builder, meta api.ResponseMeta) {
	if retryAfter, ok := humanRetryAfter(meta.RetryAfter); ok {
		if retryAfter == "" {
			b.WriteString("Retry after:\n")
		} else {
			fmt.Fprintf(b, "Retry after: %s\n", sanitizeErrorScalar(retryAfter))
		}
	}
	if rateLimit := humanRateLimit(meta.RateLimit); len(rateLimit) > 0 {
		fmt.Fprintf(b, "Rate limit: %s\n", strings.Join(rateLimit, ", "))
	}
}

// humanRetryAfter prefers the response text, including an empty header value.
// The parsed fallback keeps manually constructed metadata useful in callers
// that do not have a raw header.
func humanRetryAfter(retry api.RetryAfter) (string, bool) {
	if retry.Present || retry.Raw != "" {
		return retry.Raw, true
	}
	if retry.Wait != nil {
		return retry.Wait.String(), true
	}
	return "", false
}

// humanRateLimit prefers parsed integers and falls back to raw text in stable
// header order. Presence flags keep malformed or empty headers visible.
func humanRateLimit(rate api.RateLimit) []string {
	var entries []string
	appendEntry := func(name string, parsed *int64, raw string, present bool) {
		switch {
		case parsed != nil:
			entries = append(entries, fmt.Sprintf("%s=%d", name, *parsed))
		case present || raw != "":
			entries = append(entries, name+"="+sanitizeErrorScalar(raw))
		}
	}
	appendEntry("limit", rate.Limit, rate.RawLimit, rate.LimitPresent)
	appendEntry("remaining", rate.Remaining, rate.RawRemaining, rate.RemainingPresent)
	appendEntry("reset", rate.Reset, rate.RawReset, rate.ResetPresent)
	return entries
}

// writeHumanTransportError renders network failures without inventing a
// request id that the server never supplied.
func writeHumanTransportError(b *strings.Builder, err error) bool {
	var transportErr *api.TransportError
	if !errors.As(err, &transportErr) {
		return false
	}
	message := sanitizeErrorScalar(transportErr.Error())
	if message == "" {
		message = "api network error"
	}
	fmt.Fprintf(b, "Error: %s\n", message)
	b.WriteString("Code: api_network_error\n")
	return true
}

// humanDetailRow is one already-sanitized validation-detail line.
type humanDetailRow struct {
	key     string
	message string
}

// validationDetailRows sorts server-provided keys for stable output and drops
// rows whose key or message becomes empty after sanitation.
func validationDetailRows(details map[string][]string) []humanDetailRow {
	keys := make([]string, 0, len(details))
	for key := range details {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var rows []humanDetailRow
	for _, key := range keys {
		safeKey := sanitizeErrorScalar(key)
		for _, message := range details[key] {
			safeMessage := sanitizeErrorScalar(message)
			if safeKey == "" || safeMessage == "" {
				continue
			}
			rows = append(rows, humanDetailRow{key: safeKey, message: safeMessage})
		}
	}
	return rows
}

// rawDetailRows applies the same ordering and sanitation to scalar raw details
// recovered from bootstrap error envelopes.
func rawDetailRows(details map[string]any) []humanDetailRow {
	keys := make([]string, 0, len(details))
	for key := range details {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var rows []humanDetailRow
	for _, key := range keys {
		safeKey := sanitizeErrorScalar(key)
		safeMessage := sanitizeErrorScalar(scalarDetailText(details[key]))
		if safeKey == "" || safeMessage == "" {
			continue
		}
		rows = append(rows, humanDetailRow{key: safeKey, message: safeMessage})
	}
	return rows
}

func writeDetailRows(b *strings.Builder, rows []humanDetailRow) {
	if len(rows) == 0 {
		return
	}
	b.WriteString("Details:\n")
	for _, row := range rows {
		fmt.Fprintf(b, "  %s: %s\n", row.key, row.message)
	}
}

// writeHumanAPIErrorContext appends only display-safe credential context. The
// profile is sanitized, while the key id must be valid and unchanged by that
// sanitation before it can be shown.
func writeHumanAPIErrorContext(b *strings.Builder, err error) {
	var context *APIErrorContext
	if !errors.As(err, &context) || context == nil {
		return
	}
	if profile := sanitizeErrorScalar(context.Profile); profile != "" {
		fmt.Fprintf(b, "Profile: %s\n", profile)
	}
	if keyDisplayIDPattern.MatchString(context.KeyDisplayID) {
		if key := sanitizeErrorScalar(context.KeyDisplayID); key == context.KeyDisplayID {
			fmt.Fprintf(b, "Key: %s\n", key)
		}
	}
}

type apiErrorNotes interface {
	APIErrorNotes() []string
}

func writeHumanAPIErrorNotes(b *strings.Builder, err error) {
	var notes apiErrorNotes
	if !errors.As(err, &notes) {
		return
	}
	for _, note := range notes.APIErrorNotes() {
		if note := sanitizeErrorScalar(note); note != "" {
			fmt.Fprintf(b, "%s\n", note)
		}
	}
}

func apiErrorRemediation(code string) string {
	switch code {
	case "api_token_missing":
		return "Run chab login or set CHAB_API_KEY."
	case "invalid_api_token", "api_token_expired", "api_token_revoked", "expired_token":
		return "Refresh the stored credential with chab login."
	case "api_scope_missing", "access_denied", "team_token_required":
		return "Use a token with the required Chab scope or team access."
	case "not_found":
		return "Check the resource path and identifier."
	case "rate_limited", "temporarily_unavailable", "slow_down":
		return "Retry after the server's retry guidance when present."
	case "validation_failed", "invalid_request", "invalid_scope":
		return "Review the request fields and documented constraints."
	default:
		return ""
	}
}

// sanitizeErrorScalar turns untrusted server or local-state text into one safe
// line by redacting secrets, removing controls, and collapsing whitespace.
func sanitizeErrorScalar(value string) string {
	redacted := redact.String(value)
	sanitized := SanitizeControlBytes([]byte(redacted))
	return strings.Join(strings.Fields(string(sanitized)), " ")
}
