package output

import (
	"errors"
	"io"

	"github.com/vincentsch/chab-cli/internal/api"
)

type jsonErrorEnvelope struct {
	Error jsonErrorObject `json:"error"`
}

// jsonErrorObject declares fields in their stable compact-output order.
type jsonErrorObject struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Retryable  *bool          `json:"retryable,omitempty"`
	RequestID  string         `json:"request_id,omitempty"`
	Details    *any           `json:"details,omitempty"`
	RetryAfter *string        `json:"retry_after,omitempty"`
	RateLimit  *RateLimitView `json:"rate_limit,omitempty"`
	Recovery   *ErrorRecovery `json:"recovery,omitempty"`
}

// ErrorRecovery is optional structured guidance carried by higher-level
// commands that can safely recover from an API request failure.
type ErrorRecovery struct {
	State       string `json:"state,omitempty"`
	CanResume   bool   `json:"can_resume"`
	ResumeHint  string `json:"resume_hint,omitempty"`
	ActionID    string `json:"action_id,omitempty"`
	KnownRemote bool   `json:"known_remote"`
}

// WriteAPIErrorJSON writes one compact JSON error line when err is an
// API-client failure and reports whether it rendered. Non-API errors write
// nothing and return false.
func WriteAPIErrorJSON(w io.Writer, err error) bool {
	var protocolErr *api.ProtocolError
	if errors.As(err, &protocolErr) {
		// Protocol failures are client-side interpretations of an API response,
		// so they use a synthetic stable code but keep any captured request id.
		obj := jsonErrorObject{
			Code:      "api_protocol_error",
			Message:   protocolErr.Detail,
			RequestID: protocolErr.RequestID,
		}
		addJSONOperationalContext(&obj, protocolErr.Meta)
		addJSONRecovery(&obj, err)
		return writeJSONError(w, obj)
	}

	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		// Decoded API envelopes already carry the public error code, localized
		// message, canonical request id, and optional validation details.
		retryable := apiErr.Retryable
		details := jsonErrorDetails(apiErr)
		obj := jsonErrorObject{
			Code:      apiErr.Code,
			Message:   apiErr.Message,
			Retryable: &retryable,
			RequestID: apiErr.RequestID,
		}
		if apiErr.DetailsPresent || details != nil {
			obj.Details = &details
		}
		addJSONOperationalContext(&obj, apiErr.Meta)
		addJSONRecovery(&obj, err)
		return writeJSONError(w, obj)
	}

	var transportErr *api.TransportError
	if errors.As(err, &transportErr) {
		// Transport errors deliberately have no request id; do not fabricate one.
		obj := jsonErrorObject{
			Code:    "api_network_error",
			Message: transportErr.Error(),
		}
		addJSONRecovery(&obj, err)
		return writeJSONError(w, obj)
	}

	return false
}

type apiErrorRecovery interface {
	APIErrorRecovery() *ErrorRecovery
}

func addJSONRecovery(obj *jsonErrorObject, err error) {
	var recovery apiErrorRecovery
	if errors.As(err, &recovery) {
		obj.Recovery = recovery.APIErrorRecovery()
	}
}

// addJSONOperationalContext copies only response-backed context. A string
// pointer preserves a present empty Retry-After header instead of omitting it.
func addJSONOperationalContext(obj *jsonErrorObject, meta api.ResponseMeta) {
	if meta.RetryAfter.Present || meta.RetryAfter.Raw != "" {
		retryAfter := meta.RetryAfter.Raw
		obj.RetryAfter = &retryAfter
	}
	if rateLimit, ok := buildRateLimit(meta.RateLimit); ok {
		obj.RateLimit = rateLimit
	}
}

// jsonErrorDetails prefers Chab's fully structured details value, with older
// validation rows and bootstrap scalar details as compatibility fallbacks.
func jsonErrorDetails(apiErr *api.Error) any {
	if apiErr == nil {
		return nil
	}
	if apiErr.DetailsValue != nil {
		return apiErr.DetailsValue
	}
	if details := validationDetails(apiErr.Details); len(details) > 0 {
		return details
	}
	details := structuredRawDetails(apiErr.RawDetails)
	if len(details) == 0 {
		return nil
	}
	return details
}

// validationDetails drops empty validation rows before JSON encoding. That
// keeps an API envelope with only empty detail lists from rendering
// "details": {}.
func validationDetails(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for key, messages := range in {
		if len(messages) == 0 {
			continue
		}
		out[key] = append([]string(nil), messages...)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func writeJSONError(w io.Writer, obj jsonErrorObject) bool {
	out, err := structuredJSONBytes(jsonErrorEnvelope{Error: obj}, false)
	if err != nil {
		return false
	}
	out = append(out, '\n')
	_, err = w.Write(out)
	return err == nil
}
