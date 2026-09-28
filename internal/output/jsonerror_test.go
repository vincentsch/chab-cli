package output_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/output"
)

func TestWriteAPIErrorJSON(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantCode      string
		wantMessage   string
		wantRequestID string
		wantDetails   bool
		noRequestID   bool
	}{
		{
			name: "api error",
			err: &api.Error{
				Code:      "validation_failed",
				Message:   "Invalid.",
				RequestID: "req-1",
				Details:   map[string][]string{"name": {"required"}},
			},
			wantCode:      "validation_failed",
			wantMessage:   "Invalid.",
			wantRequestID: "req-1",
			wantDetails:   true,
		},
		{
			name:        "api error without request id",
			err:         &api.Error{Code: "invalid_api_token", Message: "Invalid."},
			wantCode:    "invalid_api_token",
			wantMessage: "Invalid.",
			noRequestID: true,
		},
		{
			name:          "protocol error",
			err:           &api.ProtocolError{Detail: "response envelope data is null", RequestID: "req-2"},
			wantCode:      "api_protocol_error",
			wantMessage:   "response envelope data is null",
			wantRequestID: "req-2",
		},
		{
			name:        "protocol error without request id",
			err:         &api.ProtocolError{Detail: "malformed JSON response body"},
			wantCode:    "api_protocol_error",
			wantMessage: "malformed JSON response body",
			noRequestID: true,
		},
		{
			name:        "transport error",
			err:         &api.TransportError{Err: errors.New("dial tcp failed"), Attempts: 1},
			wantCode:    "api_network_error",
			wantMessage: "api network error after 1 attempt(s): dial tcp failed",
			noRequestID: true,
		},
		{
			name:          "wrapped api error",
			err:           fmt.Errorf("outer: %w", &api.Error{Code: "not_found", Message: "Not found.", RequestID: "req-3"}),
			wantCode:      "not_found",
			wantMessage:   "Not found.",
			wantRequestID: "req-3",
		},
		{
			name:          "custom wrapped api error",
			err:           wrapperError{err: &api.Error{Code: "invalid_api_token", Message: "Invalid.", RequestID: "req-4"}},
			wantCode:      "invalid_api_token",
			wantMessage:   "Invalid.",
			wantRequestID: "req-4",
		},
		{
			name:          "redacted message",
			err:           &api.Error{Code: "invalid_api_token", Message: "bad key ak_test|super-secret", RequestID: "req-5"},
			wantCode:      "invalid_api_token",
			wantMessage:   "bad key ak_test|[REDACTED]",
			wantRequestID: "req-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if !output.WriteAPIErrorJSON(&buf, tt.err) {
				t.Fatalf("WriteAPIErrorJSON() = false")
			}
			if strings.Count(buf.String(), "\n") != 1 || !strings.HasSuffix(buf.String(), "\n") {
				t.Fatalf("stderr is not one JSON line: %q", buf.String())
			}
			var got struct {
				Error map[string]any `json:"error"`
			}
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("JSON parse error = %v; output=%s", err, buf.String())
			}
			if got.Error["code"] != tt.wantCode || got.Error["message"] != tt.wantMessage {
				t.Fatalf("error object = %#v", got.Error)
			}
			_, hasRequestID := got.Error["request_id"]
			if tt.noRequestID {
				if hasRequestID {
					t.Fatalf("request_id present: %#v", got.Error)
				}
			} else if got.Error["request_id"] != tt.wantRequestID {
				t.Fatalf("request_id = %#v, want %q", got.Error["request_id"], tt.wantRequestID)
			}
			if tt.wantDetails {
				if _, ok := got.Error["details"]; !ok {
					t.Fatalf("details missing: %#v", got.Error)
				}
			}
			for _, rejected := range []string{"type", "status", "retry_after", "rate_limit"} {
				if _, ok := got.Error[rejected]; ok {
					t.Fatalf("error object included rejected field %q: %#v", rejected, got.Error)
				}
			}
		})
	}
}

func TestWriteAPIErrorJSONNonAPI(t *testing.T) {
	var buf bytes.Buffer
	if output.WriteAPIErrorJSON(&buf, errors.New("local")) {
		t.Fatalf("WriteAPIErrorJSON(local) = true")
	}
	if buf.String() != "" {
		t.Fatalf("output = %q, want empty", buf.String())
	}
}

func TestWriteAPIErrorJSONPreservesExplicitNullDetails(t *testing.T) {
	var buf bytes.Buffer
	if !output.WriteAPIErrorJSON(&buf, &api.Error{
		Code:           "validation_failed",
		Message:        "Invalid.",
		Retryable:      false,
		RequestID:      "req-null",
		DetailsPresent: true,
	}) {
		t.Fatal("WriteAPIErrorJSON() = false")
	}
	if !strings.Contains(buf.String(), `"details":null`) {
		t.Fatalf("output did not preserve details:null: %s", buf.String())
	}
}

func TestWriteAPIErrorJSONIncludesRetryRateLimitAndOmitsEmptyDetails(t *testing.T) {
	limit := int64(100)
	remaining := int64(0)
	reset := int64(1770000000)
	err := &api.Error{
		Code:      "rate_limited",
		Message:   "Slow down.",
		RequestID: "req-rate",
		Details:   map[string][]string{"token": []string{}},
		Meta: api.ResponseMeta{
			RetryAfter: api.RetryAfter{Raw: "60"},
			RateLimit: api.RateLimit{
				Limit:        &limit,
				Remaining:    &remaining,
				Reset:        &reset,
				RawLimit:     "100",
				RawRemaining: "0",
				RawReset:     "1770000000",
			},
		},
	}

	var buf bytes.Buffer
	if !output.WriteAPIErrorJSON(&buf, err) {
		t.Fatalf("WriteAPIErrorJSON() = false")
	}
	var got struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v; output=%s", err, buf.String())
	}
	if got.Error["retry_after"] != "60" {
		t.Fatalf("retry_after = %#v", got.Error["retry_after"])
	}
	rateLimit, ok := got.Error["rate_limit"].(map[string]any)
	if !ok || rateLimit["limit"] != float64(100) || rateLimit["remaining"] != float64(0) || rateLimit["reset"] != float64(1770000000) {
		t.Fatalf("rate_limit = %#v", got.Error["rate_limit"])
	}
	for _, rejected := range []string{"details", "status", "type"} {
		if _, ok := got.Error[rejected]; ok {
			t.Fatalf("error object included rejected field %q: %#v", rejected, got.Error)
		}
	}
	if !strings.Contains(buf.String(), `,"request_id":"req-rate","retry_after":"60","rate_limit":`) {
		t.Fatalf("operational field order changed: %s", buf.String())
	}
}

func TestWriteAPIErrorJSONOperationalContextParityAndPresence(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "protocol present empty",
			err: &api.ProtocolError{
				Detail: "bad envelope",
				Meta: api.ResponseMeta{
					RetryAfter: api.RetryAfter{Present: true},
					RateLimit: api.RateLimit{
						LimitPresent: true,
						RawRemaining: "malformed",
					},
				},
			},
			want: `{"error":{"code":"api_protocol_error","message":"bad envelope","retry_after":"","rate_limit":{"raw":{"limit":"","remaining":"malformed"}}}}` + "\n",
		},
		{
			name: "transport remains narrow",
			err:  &api.TransportError{Err: errors.New("offline"), Attempts: 1},
			want: `{"error":{"code":"api_network_error","message":"api network error after 1 attempt(s): offline"}}` + "\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			if !output.WriteAPIErrorJSON(&buf, test.err) {
				t.Fatal("WriteAPIErrorJSON() = false")
			}
			if got := buf.String(); got != test.want {
				t.Fatalf("output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWriteAPIErrorJSONRawScalarDetailsFallback(t *testing.T) {
	err := &api.Error{
		Code:       "slow_down",
		Message:    "Slow down.",
		RequestID:  "req-slow",
		RawDetails: json.RawMessage(`{"interval":9,"note":"wait","nullable":null,"retry":true}`),
	}
	var buf bytes.Buffer
	if !output.WriteAPIErrorJSON(&buf, err) {
		t.Fatalf("WriteAPIErrorJSON() = false")
	}
	var got struct {
		Error struct {
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("JSON parse error = %v; output=%s", err, buf.String())
	}
	if got.Error.Details["interval"] != float64(9) || got.Error.Details["note"] != "wait" ||
		got.Error.Details["nullable"] != nil || got.Error.Details["retry"] != true {
		t.Fatalf("details = %#v", got.Error.Details)
	}

	buf.Reset()
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"nested":{"interval":9}}`),
		json.RawMessage(`null`),
		json.RawMessage(`[{"interval":9}]`),
		json.RawMessage(`{"interval":`),
	} {
		buf.Reset()
		err.RawDetails = raw
		if !output.WriteAPIErrorJSON(&buf, err) {
			t.Fatalf("WriteAPIErrorJSON(%s) = false", raw)
		}
		if strings.Contains(buf.String(), `"details"`) {
			t.Fatalf("raw details %s rendered unexpectedly: %s", raw, buf.String())
		}
	}
}

func TestWriteAPIErrorJSONPreservesCompactFieldOrderAndStructuredRedaction(t *testing.T) {
	err := &api.Error{
		Code:      "validation_failed",
		Message:   "bad ak_escape|part\"quoted\\control\x01雪<&tail",
		RequestID: "req-order",
		Details: map[string][]string{
			"ak_same|z": {"one"},
			"ak_same|a": {"two"},
		},
	}
	var buf bytes.Buffer
	if !output.WriteAPIErrorJSON(&buf, err) {
		t.Fatalf("WriteAPIErrorJSON() = false")
	}
	if !strings.HasPrefix(buf.String(), `{"error":{"code":"validation_failed","message":`) ||
		!strings.Contains(buf.String(), `,"request_id":"req-order","details":`) {
		t.Fatalf("compact field order changed: %s", buf.String())
	}
	if strings.Contains(buf.String(), "ak_escape|part") || strings.Contains(buf.String(), "ak_same|z") || strings.Contains(buf.String(), "ak_same|a") {
		t.Fatalf("secret survived compact error: %s", buf.String())
	}
	var decoded struct {
		Error struct {
			Details map[string][]string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("compact error is not JSON: %v\n%s", err, buf.String())
	}
	base := "ak_same|[REDACTED]"
	if got := decoded.Error.Details[base]; len(got) != 1 || got[0] != "two" {
		t.Fatalf("base collision value = %#v", got)
	}
	if got := decoded.Error.Details[base+"~2"]; len(got) != 1 || got[0] != "one" {
		t.Fatalf("suffixed collision value = %#v", got)
	}

	buf.Reset()
	if !output.WriteAPIErrorJSON(&buf, &api.Error{Code: "invalid_api_token", Message: "Invalid.", RequestID: "req"}) {
		t.Fatalf("secret-free WriteAPIErrorJSON() = false")
	}
	if got, want := buf.String(), "{\"error\":{\"code\":\"invalid_api_token\",\"message\":\"Invalid.\",\"retryable\":false,\"request_id\":\"req\"}}\n"; got != want {
		t.Fatalf("secret-free compact bytes = %q, want %q", got, want)
	}
}

func TestWriteAPIErrorJSONPreservesEncodedValuesAndExactRawNumbers(t *testing.T) {
	err := &api.Error{
		Code:      "validation_failed",
		Message:   "Invalid.",
		RequestID: "req-raw",
		RawDetails: json.RawMessage(`{
			"huge":184467440737095516160,
			"encoded":"quote\"slash\\control\u0001雪<& ak_encoded%7Cencoded-secret tail",
			"literal":"ak_literal|literal-secret tail"
		}`),
	}

	var buf bytes.Buffer
	if !output.WriteAPIErrorJSON(&buf, err) {
		t.Fatalf("WriteAPIErrorJSON() = false")
	}
	if strings.Count(buf.String(), "\n") != 1 || !strings.HasSuffix(buf.String(), "\n") {
		t.Fatalf("stderr is not one compact JSON line: %q", buf.String())
	}
	for _, secret := range []string{
		"ak_encoded%7Cencoded-secret",
		"encoded-secret",
		"ak_literal|literal-secret",
		"literal-secret",
	} {
		if strings.Contains(buf.String(), secret) {
			t.Fatalf("compact error contains secret %q: %s", secret, buf.String())
		}
	}

	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.UseNumber()
	var decoded struct {
		Error struct {
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := dec.Decode(&decoded); err != nil {
		t.Fatalf("compact error is not JSON: %v\n%s", err, buf.String())
	}
	if got := decoded.Error.Details["huge"]; got != json.Number("184467440737095516160") {
		t.Fatalf("huge detail = %#v", got)
	}
	if got, want := decoded.Error.Details["encoded"], "quote\"slash\\control\x01雪<& ak_encoded%7C[REDACTED] tail"; got != want {
		t.Fatalf("encoded detail = %#v, want %#v", got, want)
	}
	if got, want := decoded.Error.Details["literal"], "ak_literal|[REDACTED] tail"; got != want {
		t.Fatalf("literal detail = %#v, want %#v", got, want)
	}
}

type wrapperError struct {
	err error
}

func (e wrapperError) Error() string {
	return "wrapped: " + e.err.Error()
}

func (e wrapperError) Unwrap() error {
	return e.err
}
