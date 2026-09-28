package output_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/output"
)

func TestWrapMetaBuildsStableSuccessEnvelope(t *testing.T) {
	limit := int64(100)
	remaining := int64(0)
	reset := int64(1770000000)
	from := 1
	to := 2
	replayed := true
	wrapped := output.WrapMeta(
		[]map[string]string{{"id": "one"}},
		api.ResponseMeta{
			RequestID:          "req-header",
			HeaderRequestID:    "req-header",
			EnvelopeRequestID:  "req-envelope",
			RateLimit:          api.RateLimit{Limit: &limit, Remaining: &remaining, Reset: &reset, RawLimit: "100", RawRemaining: "0", RawReset: "1770000000"},
			Attempts:           3,
			RetryWaits:         []time.Duration{10 * time.Millisecond, 25 * time.Millisecond},
			IdempotencyUsed:    true,
			IdempotentReplayed: &replayed,
			Pagination:         &api.Pagination{CurrentPage: 2, PerPage: 100, Total: 101, LastPage: 2, From: &from, To: &to, HasMore: false},
			RawMeta: map[string]json.RawMessage{
				"pagination": json.RawMessage(`{"current_page":2}`),
				"server":     json.RawMessage(`"v1"`),
			},
		},
		output.MetaOptions{IncludeAPIMeta: true},
	)

	raw, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got struct {
		Data []map[string]string `json:"data"`
		Meta struct {
			RequestID  string `json:"request_id"`
			RequestIDs struct {
				Header   string `json:"header"`
				Envelope string `json:"envelope"`
			} `json:"request_ids"`
			Pagination struct {
				CurrentPage int `json:"current_page"`
				Total       int `json:"total"`
			} `json:"pagination"`
			RateLimit struct {
				Limit     int64             `json:"limit"`
				Remaining int64             `json:"remaining"`
				Reset     int64             `json:"reset"`
				Raw       map[string]string `json:"raw"`
			} `json:"rate_limit"`
			Retry struct {
				Attempts int     `json:"attempts"`
				WaitsMS  []int64 `json:"waits_ms"`
			} `json:"retry"`
			IdempotentReplayed bool                       `json:"idempotent_replayed"`
			APIMeta            map[string]json.RawMessage `json:"api_meta"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v; output=%s", err, raw)
	}
	if got.Data[0]["id"] != "one" || got.Meta.RequestID != "req-header" {
		t.Fatalf("basic wrapper = %#v", got)
	}
	if got.Meta.RequestIDs.Header != "req-header" || got.Meta.RequestIDs.Envelope != "req-envelope" {
		t.Fatalf("request ids = %#v", got.Meta.RequestIDs)
	}
	if got.Meta.Pagination.CurrentPage != 2 || got.Meta.Pagination.Total != 101 {
		t.Fatalf("pagination = %#v", got.Meta.Pagination)
	}
	if got.Meta.RateLimit.Limit != 100 || got.Meta.RateLimit.Remaining != 0 || got.Meta.RateLimit.Raw["reset"] != "1770000000" {
		t.Fatalf("rate limit = %#v", got.Meta.RateLimit)
	}
	if got.Meta.Retry.Attempts != 3 || len(got.Meta.Retry.WaitsMS) != 2 || got.Meta.Retry.WaitsMS[1] != 25 {
		t.Fatalf("retry = %#v", got.Meta.Retry)
	}
	if !got.Meta.IdempotentReplayed {
		t.Fatalf("idempotent_replayed = false")
	}
	if string(got.Meta.APIMeta["server"]) != `"v1"` || got.Meta.APIMeta["pagination"] != nil {
		t.Fatalf("api_meta = %#v", got.Meta.APIMeta)
	}
}

func TestWrapMetaOmitsUnavailableFields(t *testing.T) {
	raw, err := json.Marshal(output.WrapMeta(map[string]bool{"ok": true}, api.ResponseMeta{}, output.MetaOptions{IncludeAPIMeta: true}))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(raw) != `{"data":{"ok":true},"meta":{}}` {
		t.Fatalf("wrapped = %s", raw)
	}
}

func TestWrapMetaPreservesHeaderPresenceAndReplayFalse(t *testing.T) {
	replayed := false
	raw, err := json.Marshal(output.WrapMeta("value", api.ResponseMeta{
		RateLimit: api.RateLimit{
			LimitPresent:     true,
			RawRemaining:     "malformed",
			RemainingPresent: true,
			ResetPresent:     true,
		},
		IdempotencyUsed:    true,
		IdempotentReplayed: &replayed,
	}, output.MetaOptions{}))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want := `{"data":"value","meta":{"rate_limit":{"raw":{"limit":"","remaining":"malformed","reset":""}},"idempotent_replayed":false}}`
	if got := string(raw); got != want {
		t.Fatalf("wrapped = %s, want %s", got, want)
	}
}

func TestWrapMetaPreservesParsedOnlyRateLimitCompatibility(t *testing.T) {
	limit := int64(12)
	raw, err := json.Marshal(output.WrapMeta(
		json.RawMessage(`9007199254740993`),
		api.ResponseMeta{
			RateLimit: api.RateLimit{Limit: &limit},
			RawMeta: map[string]json.RawMessage{
				"pagination": json.RawMessage(`{"current_page":1}`),
			},
		},
		output.MetaOptions{IncludeAPIMeta: true},
	))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want := `{"data":9007199254740993,"meta":{"rate_limit":{"limit":12}}}`
	if got := string(raw); got != want {
		t.Fatalf("wrapped = %s, want %s", got, want)
	}
}
