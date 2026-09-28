package output

import (
	"encoding/json"

	"github.com/vincentsch/chab-cli/internal/api"
)

// MetaOptions controls optional metadata fields in the success wrapper.
type MetaOptions struct {
	// IncludeAPIMeta controls whether raw envelope meta is emitted as api_meta.
	IncludeAPIMeta bool
}

// metaEnvelope is the stable top-level shape produced by --include-meta.
// Commands keep their normal machine value under data and expose response
// metadata separately under meta.
type metaEnvelope struct {
	Data any      `json:"data"`
	Meta MetaView `json:"meta"`
}

// MetaView is intentionally separate from api.ResponseMeta. ResponseMeta keeps
// internal transport details, while this view contains only the public scripting
// fields that are safe and stable to serialize.
type MetaView struct {
	RequestID          string                     `json:"request_id,omitempty"`
	RequestIDs         *RequestIDsView            `json:"request_ids,omitempty"`
	Pagination         *api.Pagination            `json:"pagination,omitempty"`
	Cursor             *api.CursorPagination      `json:"cursor,omitempty"`
	RateLimit          *RateLimitView             `json:"rate_limit,omitempty"`
	Retry              *RetryView                 `json:"retry,omitempty"`
	IdempotentReplayed *bool                      `json:"idempotent_replayed,omitempty"`
	APIMeta            map[string]json.RawMessage `json:"api_meta,omitempty"`
}

type RequestIDsView struct {
	Header   string `json:"header"`
	Envelope string `json:"envelope"`
}

// RateLimitView is used by success metadata to expose parsed values and raw
// header text.
type RateLimitView struct {
	Limit     *int64            `json:"limit,omitempty"`
	Remaining *int64            `json:"remaining,omitempty"`
	Reset     *int64            `json:"reset,omitempty"`
	Raw       map[string]string `json:"raw,omitempty"`
}

// RetryView reports physical request attempts, not logical page count. For
// paginated commands, cmdutil.FetchPages aggregates this before rendering.
type RetryView struct {
	Attempts int     `json:"attempts"`
	WaitsMS  []int64 `json:"waits_ms,omitempty"`
}

// WrapMeta returns the stable {data, meta} success envelope used by
// --include-meta machine output.
func WrapMeta(data any, meta api.ResponseMeta, opts MetaOptions) any {
	return metaEnvelope{Data: data, Meta: ViewMeta(meta, opts)}
}

// ViewMeta returns the stable safe response metadata view without wrapping a
// command's primary machine value.
func ViewMeta(meta api.ResponseMeta, opts MetaOptions) MetaView {
	view := MetaView{
		RequestID:  meta.RequestID,
		Pagination: meta.Pagination,
		Cursor:     meta.CursorPagination,
	}
	if meta.HeaderRequestID != "" && meta.EnvelopeRequestID != "" && meta.HeaderRequestID != meta.EnvelopeRequestID {
		view.RequestIDs = &RequestIDsView{
			Header:   meta.HeaderRequestID,
			Envelope: meta.EnvelopeRequestID,
		}
	}
	if rateLimit, ok := buildRateLimit(meta.RateLimit); ok {
		view.RateLimit = rateLimit
	}
	if meta.Attempts > 0 {
		retry := &RetryView{Attempts: meta.Attempts}
		if len(meta.RetryWaits) > 0 {
			retry.WaitsMS = make([]int64, 0, len(meta.RetryWaits))
			for _, wait := range meta.RetryWaits {
				retry.WaitsMS = append(retry.WaitsMS, wait.Milliseconds())
			}
		}
		view.Retry = retry
	}
	if meta.IdempotencyUsed {
		// Replay state is meaningful only when the request sent a key. A missing
		// or explicit false response header is exposed as false.
		replayed := meta.IdempotentReplayed != nil && *meta.IdempotentReplayed
		view.IdempotentReplayed = &replayed
	}
	if opts.IncludeAPIMeta && len(meta.RawMeta) > 0 {
		apiMeta := make(map[string]json.RawMessage, len(meta.RawMeta))
		for key, value := range meta.RawMeta {
			if key == "pagination" {
				// Pagination has a typed field above; keeping it in api_meta would
				// give scripts two places to read the same value.
				continue
			}
			apiMeta[key] = value
		}
		if len(apiMeta) > 0 {
			view.APIMeta = apiMeta
		}
	}
	return view
}

// Empty reports whether the view has no public fields to serialize.
func (v MetaView) Empty() bool {
	return v.RequestID == "" &&
		v.RequestIDs == nil &&
		v.Pagination == nil &&
		v.Cursor == nil &&
		v.RateLimit == nil &&
		v.Retry == nil &&
		v.IdempotentReplayed == nil &&
		len(v.APIMeta) == 0
}

// buildRateLimit returns nil when no rate-limit headers were captured. Parsed
// integers and raw strings are both kept because malformed and present-empty
// headers are still useful diagnostic context.
func buildRateLimit(rl api.RateLimit) (*RateLimitView, bool) {
	if rl.Limit == nil && rl.Remaining == nil && rl.Reset == nil &&
		rl.RawLimit == "" && rl.RawRemaining == "" && rl.RawReset == "" &&
		!rl.LimitPresent && !rl.RemainingPresent && !rl.ResetPresent {
		return nil, false
	}

	view := &RateLimitView{
		Limit:     rl.Limit,
		Remaining: rl.Remaining,
		Reset:     rl.Reset,
	}
	raw := make(map[string]string)
	if rl.LimitPresent || rl.RawLimit != "" {
		raw["limit"] = rl.RawLimit
	}
	if rl.RemainingPresent || rl.RawRemaining != "" {
		raw["remaining"] = rl.RawRemaining
	}
	if rl.ResetPresent || rl.RawReset != "" {
		raw["reset"] = rl.RawReset
	}
	if len(raw) > 0 {
		view.Raw = raw
	}
	return view, true
}
