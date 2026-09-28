package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vincentsch/chab-cli/internal/redact"
)

// ResponseMeta carries response headers, retry metadata, and decoded envelope
// metadata needed by renderers and paginated request execution.
type ResponseMeta struct {
	HTTPStatus        int
	RequestID         string
	HeaderRequestID   string
	EnvelopeRequestID string
	RateLimit         RateLimit
	RetryAfter        RetryAfter
	// IdempotencyUsed is request-side metadata: it records whether this request
	// sent an idempotency key, even when the response has no replay header.
	IdempotencyUsed bool
	// IdempotentReplayed is response-side metadata. Nil means the API did not
	// send the header, while false means it sent an explicit non-replayed value.
	IdempotentReplayed *bool
	// IdempotencyOutcome is a backend marker on downstream error responses.
	// A released or settled non-2xx was processed by idempotency admission; an
	// absent marker leaves a pre-idempotency throttle outcome uncertain.
	IdempotencyOutcome string
	Attempts           int
	RetryWaits         []time.Duration
	Pagination         *Pagination
	CursorPagination   *CursorPagination
	traversalCursor    *CursorPagination
	RawMeta            map[string]json.RawMessage
	// traversalPagination retains validated response pagination for request
	// execution when the public Pagination view must be suppressed by
	// request-local redaction. It is deliberately unexported so generic
	// serialization and output renderers cannot expose its source values.
	traversalPagination *Pagination
	// errorRedactor retains the request-local sanitation scope for failures
	// raised after a successful response handoff, such as jq/template
	// evaluation and stdout writes. It is deliberately private so successful
	// API-owned data remains outside diagnostic redaction.
	errorRedactor *requestRedactor
}

// RateLimit preserves parsed and raw rate-limit header values. Presence flags
// distinguish a missing header from a present header with an empty value.
type RateLimit struct {
	Limit            *int64
	Remaining        *int64
	Reset            *int64
	RawLimit         string
	RawRemaining     string
	RawReset         string
	LimitPresent     bool
	RemainingPresent bool
	ResetPresent     bool
}

// RetryAfter preserves parsed and raw Retry-After values. Present distinguishes
// a missing header from a present empty value.
type RetryAfter struct {
	Wait    *time.Duration
	Raw     string
	Present bool
}

// PaginationForTraversal returns validated server pagination for internal page
// execution. Manually constructed metadata falls back to the public carrier.
func (m ResponseMeta) PaginationForTraversal() *Pagination {
	if m.traversalPagination != nil {
		return m.traversalPagination
	}
	return m.Pagination
}

// CursorForTraversal is execution-only; diagnostic redaction does not alter it.
func (m ResponseMeta) CursorForTraversal() *CursorPagination {
	if m.traversalCursor != nil {
		return m.traversalCursor
	}
	return m.CursorPagination
}

// RedactError applies the response's request-local diagnostic scope while
// preserving the original error in the unwrap chain.
func (m ResponseMeta) RedactError(err error) error {
	if err == nil || m.errorRedactor == nil {
		return err
	}
	return m.errorRedactor.redactErr(err)
}

// RedactJSON applies the request-local redaction scope that produced this
// metadata to a raw successful response value. Raw API commands use it for
// request-marked secrets and idempotency keys that a server echoes back.
func (m ResponseMeta) RedactJSON(raw json.RawMessage) (json.RawMessage, error) {
	transform := redact.String
	if m.errorRedactor != nil {
		transform = m.errorRedactor.redactText
	}
	sanitized, err := redact.TransformOrderedJSON(raw, transform)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(sanitized), nil
}

// captureResponseMeta keeps both parsed values and raw header text. Raw values
// are useful when a header is present but malformed or when future output modes
// need to show exactly what the API sent.
func captureResponseMeta(status int, header http.Header, now time.Time) ResponseMeta {
	headerRequestID, _ := firstHeader(header, "X-Request-Id")
	meta := ResponseMeta{
		HTTPStatus:      status,
		HeaderRequestID: headerRequestID,
	}
	meta.RequestID = meta.HeaderRequestID

	if raw, ok := firstHeader(header, "X-RateLimit-Limit"); ok {
		meta.RateLimit.LimitPresent = true
		meta.RateLimit.RawLimit = raw
		meta.RateLimit.Limit = parseInt64Header(raw)
	}
	if raw, ok := firstHeader(header, "X-RateLimit-Remaining"); ok {
		meta.RateLimit.RemainingPresent = true
		meta.RateLimit.RawRemaining = raw
		meta.RateLimit.Remaining = parseInt64Header(raw)
	}
	if raw, ok := firstHeader(header, "X-RateLimit-Reset"); ok {
		meta.RateLimit.ResetPresent = true
		meta.RateLimit.RawReset = raw
		meta.RateLimit.Reset = parseInt64Header(raw)
	}
	if raw, ok := firstHeader(header, "Retry-After"); ok {
		meta.RetryAfter.Present = true
		meta.RetryAfter.Raw = raw
		meta.RetryAfter.Wait = parseRetryAfter(raw, now)
	}
	if raw, ok := firstHeader(header, "Idempotent-Replayed"); ok {
		replayed := strings.EqualFold(strings.TrimSpace(raw), "true")
		meta.IdempotentReplayed = &replayed
	}
	if raw, ok := firstHeader(header, "X-Chab-Idempotency-Outcome"); ok {
		meta.IdempotencyOutcome = strings.ToLower(strings.TrimSpace(raw))
	}
	return meta
}

// redactResponseMeta sanitizes response-owned text before metadata crosses an
// API handoff. Parsed header values are safe to retain only when their original
// source text was unchanged by the request-local transform.
func redactResponseMeta(meta ResponseMeta, transform func(string) string) ResponseMeta {
	meta.RequestID = transform(meta.RequestID)
	meta.HeaderRequestID = transform(meta.HeaderRequestID)
	meta.EnvelopeRequestID = transform(meta.EnvelopeRequestID)
	meta.IdempotencyOutcome = transform(meta.IdempotencyOutcome)

	redactParsedInt := func(raw string, parsed **int64) string {
		redacted := transform(raw)
		if redacted != raw {
			*parsed = nil
		}
		return redacted
	}
	meta.RateLimit.RawLimit = redactParsedInt(meta.RateLimit.RawLimit, &meta.RateLimit.Limit)
	meta.RateLimit.RawRemaining = redactParsedInt(meta.RateLimit.RawRemaining, &meta.RateLimit.Remaining)
	meta.RateLimit.RawReset = redactParsedInt(meta.RateLimit.RawReset, &meta.RateLimit.Reset)

	rawRetryAfter := meta.RetryAfter.Raw
	meta.RetryAfter.Raw = transform(rawRetryAfter)
	if meta.RetryAfter.Raw != rawRetryAfter {
		meta.RetryAfter.Wait = nil
	}
	return meta
}

// firstHeader handles canonical and non-canonical maps so tests and custom
// transports do not need to mimic net/http's exact header normalization.
func firstHeader(header http.Header, name string) (string, bool) {
	if values, ok := header[http.CanonicalHeaderKey(name)]; ok && len(values) > 0 {
		return values[0], true
	}
	for key, values := range header {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0], true
		}
	}
	return "", false
}

func parseInt64Header(raw string) *int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return nil
	}
	return &value
}

// parseRetryAfter accepts the two HTTP forms: delta seconds and HTTP-date. A
// past HTTP-date means "retry now"; malformed values are left as raw metadata.
func parseRetryAfter(raw string, now time.Time) *time.Duration {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	if allDigits(trimmed) {
		seconds, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return nil
		}
		const maxDurationSeconds = int64(1<<63-1) / int64(time.Second)
		if seconds > maxDurationSeconds {
			return nil
		}
		wait := time.Duration(seconds) * time.Second
		return &wait
	}

	when, err := http.ParseTime(trimmed)
	if err != nil {
		return nil
	}
	wait := when.Sub(now)
	if wait < 0 {
		wait = 0
	}
	return &wait
}

func allDigits(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return value != ""
}
