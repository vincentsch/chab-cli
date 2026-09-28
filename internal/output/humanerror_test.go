package output_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/output"
)

func int64Pointer(value int64) *int64 {
	return &value
}

func TestWriteAPIErrorHumanAPIErrorWithContext(t *testing.T) {
	const keyDisplayID = "ak_01ARZ3NDEKTSV4RRFFQ69G5FAV"
	meta := api.ResponseMeta{
		RetryAfter: api.RetryAfter{Raw: "30"},
		RateLimit: api.RateLimit{
			RawLimit:     "100",
			RawRemaining: "0",
			RawReset:     "1710000000",
		},
	}
	err := output.WithCredentialContext(&api.Error{
		Code:      "validation_failed",
		Message:   "Invalid project.",
		RequestID: "req-1",
		Details: map[string][]string{
			"name":   {"is required"},
			"status": {"is unsupported", "must be active"},
		},
		Meta: meta,
	}, "production", keyDisplayID)

	var buf bytes.Buffer
	if !output.WriteAPIErrorHuman(&buf, err) {
		t.Fatalf("WriteAPIErrorHuman() = false")
	}
	want := "" +
		"Error: Invalid project.\n" +
		"Code: validation_failed\n" +
		"Retryable: false\n" +
		"Request ID: req-1\n" +
		"Details:\n" +
		"  name: is required\n" +
		"  status: is unsupported\n" +
		"  status: must be active\n" +
		"Remediation: Review the request fields and documented constraints.\n" +
		"Retry after: 30\n" +
		"Rate limit: limit=100, remaining=0, reset=1710000000\n" +
		"Profile: production\n" +
		"Key: " + keyDisplayID + "\n"
	if buf.String() != want {
		t.Fatalf("human error = %q, want %q", buf.String(), want)
	}
}

func TestWriteAPIErrorHumanOperationalContextParityAndPresence(t *testing.T) {
	wait := 1500 * time.Millisecond
	tests := []struct {
		name string
		err  error
		want []string
	}{
		{
			name: "protocol present empty and malformed",
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
			want: []string{"Retry after:\n", "Rate limit: limit=, remaining=malformed\n"},
		},
		{
			name: "parsed only fallback",
			err: &api.Error{
				Code:    "rate_limited",
				Message: "wait",
				Meta: api.ResponseMeta{
					RetryAfter: api.RetryAfter{Wait: &wait},
					RateLimit:  api.RateLimit{Limit: int64Pointer(12)},
				},
			},
			want: []string{"Retry after: 1.5s\n", "Rate limit: limit=12\n"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			if !output.WriteAPIErrorHuman(&buf, test.err) {
				t.Fatal("WriteAPIErrorHuman() = false")
			}
			for _, want := range test.want {
				if !strings.Contains(buf.String(), want) {
					t.Fatalf("output missing %q:\n%s", want, buf.String())
				}
			}
		})
	}
}

func TestWriteAPIErrorHumanRawScalarDetailsFallback(t *testing.T) {
	err := &api.Error{
		Code:       "slow_down",
		Message:    "Slow down.",
		RequestID:  "req-slow",
		RawDetails: json.RawMessage(`{"interval":9,"note":"wait","nullable":null,"retry":true}`),
	}
	var buf bytes.Buffer
	if !output.WriteAPIErrorHuman(&buf, err) {
		t.Fatalf("WriteAPIErrorHuman() = false")
	}
	want := "" +
		"Error: Slow down.\n" +
		"Code: slow_down\n" +
		"Retryable: false\n" +
		"Request ID: req-slow\n" +
		"Details:\n" +
		"  interval: 9\n" +
		"  note: wait\n" +
		"  nullable: null\n" +
		"  retry: true\n" +
		"Remediation: Retry after the server's retry guidance when present.\n"
	if buf.String() != want {
		t.Fatalf("human error = %q, want %q", buf.String(), want)
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
		if !output.WriteAPIErrorHuman(&buf, err) {
			t.Fatalf("WriteAPIErrorHuman(%s) = false", raw)
		}
		if strings.Contains(buf.String(), "Details:") {
			t.Fatalf("raw details %s rendered unexpectedly:\n%s", raw, buf.String())
		}
	}
}

func TestWriteAPIErrorHumanOmitsEmptyValidationDetails(t *testing.T) {
	err := &api.Error{
		Code:    "validation_failed",
		Message: "Invalid.",
		Details: map[string][]string{
			"name": {},
		},
	}
	var buf bytes.Buffer
	if !output.WriteAPIErrorHuman(&buf, err) {
		t.Fatalf("WriteAPIErrorHuman() = false")
	}
	if strings.Contains(buf.String(), "Details:") {
		t.Fatalf("empty validation details rendered unexpectedly:\n%s", buf.String())
	}
}

func TestWriteAPIErrorHumanOmitsEmptyKeyContext(t *testing.T) {
	err := output.WithCredentialContext(&api.Error{
		Code:      "forbidden_ability",
		Message:   "No access.",
		RequestID: "req-2",
	}, "local", "")

	var buf bytes.Buffer
	if !output.WriteAPIErrorHuman(&buf, err) {
		t.Fatalf("WriteAPIErrorHuman() = false")
	}
	for _, want := range []string{"Error: No access.", "Code: forbidden_ability", "Request ID: req-2"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, buf.String())
		}
	}
	if !strings.Contains(buf.String(), "Profile: local\n") || strings.Contains(buf.String(), "Key:") {
		t.Fatalf("unexpected credential context:\n%s", buf.String())
	}
	assertHumanErrorOmitsOperationalContext(t, buf.String())
}

func TestWriteAPIErrorHumanProtocolAndTransport(t *testing.T) {
	const keyDisplayID = "ak_01ARZ3NDEKTSV4RRFFQ69G5FAV"
	protocol := output.WithCredentialContext(&api.ProtocolError{
		Detail:    "response envelope data is null",
		RequestID: "req-protocol",
	}, "staging", keyDisplayID)

	var buf bytes.Buffer
	if !output.WriteAPIErrorHuman(&buf, protocol) {
		t.Fatalf("protocol WriteAPIErrorHuman() = false")
	}
	for _, want := range []string{
		"Error: response envelope data is null",
		"Code: api_protocol_error",
		"Request ID: req-protocol",
		"Profile: staging",
		"Key: " + keyDisplayID,
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("protocol output missing %q:\n%s", want, buf.String())
		}
	}
	assertHumanErrorOmitsOperationalContext(t, buf.String())

	buf.Reset()
	transport := output.WithCredentialContext(&api.TransportError{Err: errors.New("dial tcp failed"), Attempts: 1}, "staging", keyDisplayID)
	if !output.WriteAPIErrorHuman(&buf, transport) {
		t.Fatalf("transport WriteAPIErrorHuman() = false")
	}
	for _, want := range []string{
		"Error: api network error after 1 attempt(s): dial tcp failed",
		"Code: api_network_error",
		"Profile: staging",
		"Key: " + keyDisplayID,
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("transport output missing %q:\n%s", want, buf.String())
		}
	}
	if strings.Contains(buf.String(), "Request ID:") {
		t.Fatalf("transport output included request id:\n%s", buf.String())
	}
	assertHumanErrorOmitsOperationalContext(t, buf.String())
}

func TestWriteAPIErrorHumanUnwrappedTypedErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "api", err: &api.Error{Code: "invalid_api_token", Message: "Invalid."}, code: "invalid_api_token"},
		{name: "protocol", err: &api.ProtocolError{Detail: "malformed JSON response body"}, code: "api_protocol_error"},
		{name: "transport", err: &api.TransportError{Err: errors.New("EOF"), Attempts: 2}, code: "api_network_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if !output.WriteAPIErrorHuman(&buf, tt.err) {
				t.Fatalf("WriteAPIErrorHuman() = false")
			}
			if !strings.Contains(buf.String(), "Code: "+tt.code) {
				t.Fatalf("output = %q", buf.String())
			}
			if strings.Contains(buf.String(), "Profile:") || strings.Contains(buf.String(), "Key:") {
				t.Fatalf("unwrapped output included context:\n%s", buf.String())
			}
		})
	}
}

func TestWriteAPIErrorHumanNonAPI(t *testing.T) {
	var buf bytes.Buffer
	if output.WriteAPIErrorHuman(&buf, errors.New("local")) {
		t.Fatalf("WriteAPIErrorHuman(local) = true")
	}
	if buf.String() != "" {
		t.Fatalf("output = %q, want empty", buf.String())
	}
}

func TestWriteAPIErrorJSONThroughCredentialContextOmitsContext(t *testing.T) {
	err := output.WithCredentialContext(&api.Error{
		Code:      "not_found",
		Message:   "Not found.",
		RequestID: "req-json",
	}, "production", "ak_01ARZ3NDEKTSV4RRFFQ69G5FAV")

	var buf bytes.Buffer
	if !output.WriteAPIErrorJSON(&buf, err) {
		t.Fatalf("WriteAPIErrorJSON() = false")
	}
	var got struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("JSON parse error = %v; output=%s", err, buf.String())
	}
	if got.Error["code"] != "not_found" || got.Error["request_id"] != "req-json" {
		t.Fatalf("error object = %#v", got.Error)
	}
	if _, ok := got.Error["profile"]; ok {
		t.Fatalf("JSON included profile: %#v", got.Error)
	}
	if _, ok := got.Error["key"]; ok {
		t.Fatalf("JSON included key: %#v", got.Error)
	}
	if strings.Contains(buf.String(), "production") || strings.Contains(buf.String(), "ak_01ARZ3NDEKTSV4RRFFQ69G5FAV") {
		t.Fatalf("JSON leaked context: %s", buf.String())
	}
}

func TestWriteAPIErrorHumanAcceptsOnlyValidKeyDisplayIDs(t *testing.T) {
	const valid = "ak_01ARZ3NDEKTSV4RRFFQ69G5FAV"
	const uuid = "5f727d98-7ed2-47f9-b69e-b3d975d5654b"
	invalidUTF8 := string([]byte{'a', 'k', '_', 0xff, 'A'})
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "valid", key: valid, want: true},
		{name: "uuid", key: uuid, want: true},
		{name: "lowercase", key: strings.ToLower(valid), want: true},
		{name: "wrong length", key: valid[:len(valid)-1], want: true},
		{name: "whitespace padded", key: " " + valid + " "},
		{name: "control bearing", key: valid[:10] + "\n" + valid[10:]},
		{name: "ansi bearing", key: "\x1b[31m" + valid},
		{name: "malformed utf8", key: invalidUTF8},
		{name: "token shaped", key: "ak_demo|secret"},
		{name: "empty", key: ""},
		{name: "short fake", key: "ak_display", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := output.WithCredentialContext(&api.Error{Code: "invalid_api_token", Message: "Invalid."}, "profile", tt.key)
			var buf bytes.Buffer
			if !output.WriteAPIErrorHuman(&buf, err) {
				t.Fatalf("WriteAPIErrorHuman() = false")
			}
			if !strings.Contains(buf.String(), "Profile: profile\n") {
				t.Fatalf("profile missing:\n%s", buf.String())
			}
			hasKey := strings.Contains(buf.String(), "Key:")
			if hasKey != tt.want {
				t.Fatalf("Key line present = %t, want %t:\n%s", hasKey, tt.want, buf.String())
			}
			if strings.Contains(buf.String(), "secret") || strings.Contains(buf.String(), "\x1b") || strings.Contains(buf.String(), string([]byte{0xff})) {
				t.Fatalf("unsafe key bytes survived:\n%q", buf.String())
			}
		})
	}
}

func TestWriteAPIErrorHumanSanitizesEveryScalarAndRetainsDetailCollisions(t *testing.T) {
	err := output.WithCredentialContext(&api.Error{
		Code:      "bad\nCode: injected",
		Message:   "\x1b[31mBad\r\nError: injected\tmessage\x00",
		RequestID: "req\nProfile: fake",
		Details: map[string][]string{
			"a\tKey: fake": {"first\nDetails: fake"},
			"a\nKey: fake": {"second\rCode: fake"},
			"\x1b[31m":     {"dropped"},
			"empty":        {"\x1b[31m"},
		},
	}, "prod\nKey: fake", "ak_display")

	var buf bytes.Buffer
	if !output.WriteAPIErrorHuman(&buf, err) {
		t.Fatalf("WriteAPIErrorHuman() = false")
	}
	want := "" +
		"Error: Bad Error: injected message\n" +
		"Code: bad Code: injected\n" +
		"Retryable: false\n" +
		"Request ID: req Profile: fake\n" +
		"Details:\n" +
		"  a Key: fake: first Details: fake\n" +
		"  a Key: fake: second Code: fake\n" +
		"Profile: prod Key: fake\n" +
		"Key: ak_display\n"
	if buf.String() != want {
		t.Fatalf("human error = %q, want %q", buf.String(), want)
	}
	if strings.Contains(buf.String(), "\r") || strings.Contains(buf.String(), "\t") || strings.Contains(buf.String(), "\x1b") || strings.Contains(buf.String(), "\x00") {
		t.Fatalf("unsafe controls survived: %q", buf.String())
	}
}

func TestWriteAPIErrorHumanSanitizesRawProtocolAndTransportScalars(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    string
		secrets []string
	}{
		{
			name: "raw details",
			err: &api.Error{
				Code:       "raw_error",
				Message:    "Raw failure",
				RequestID:  "req-raw",
				RawDetails: json.RawMessage(`{"raw\nkey":"line\r\ninjected\t\u001b[31mred ak_raw|raw-secret tail"}`),
			},
			want: "" +
				"Error: Raw failure\n" +
				"Code: raw_error\n" +
				"Retryable: false\n" +
				"Request ID: req-raw\n" +
				"Details:\n" +
				"  raw key: line injected red ak_raw|[REDACTED] tail\n",
			secrets: []string{"ak_raw|raw-secret", "raw-secret"},
		},
		{
			name: "protocol",
			err: output.WithCredentialContext(&api.ProtocolError{
				Detail:    "protocol\nCode: forged\t\x1b[31mred ak_protocol|protocol-secret tail",
				RequestID: "req\r\nProfile: forged",
			}, "safe", ""),
			want: "" +
				"Error: protocol Code: forged red ak_protocol|[REDACTED] tail\n" +
				"Code: api_protocol_error\n" +
				"Request ID: req Profile: forged\n" +
				"Profile: safe\n",
			secrets: []string{"ak_protocol|protocol-secret", "protocol-secret"},
		},
		{
			name: "transport",
			err: output.WithCredentialContext(&api.TransportError{
				Err:      errors.New("dial failed\nCode: forged\t\x1b[31mred ak_transport|transport-secret tail"),
				Attempts: 1,
			}, "safe", ""),
			want: "" +
				"Error: api network error after 1 attempt(s): dial failed Code: forged red ak_transport|[REDACTED] tail\n" +
				"Code: api_network_error\n" +
				"Profile: safe\n",
			secrets: []string{"ak_transport|transport-secret", "transport-secret"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if !output.WriteAPIErrorHuman(&buf, tt.err) {
				t.Fatalf("WriteAPIErrorHuman() = false")
			}
			if buf.String() != tt.want {
				t.Fatalf("human error = %q, want %q", buf.String(), tt.want)
			}
			for _, secret := range tt.secrets {
				if strings.Contains(buf.String(), secret) {
					t.Fatalf("human error contains secret %q: %q", secret, buf.String())
				}
			}
			if strings.Contains(buf.String(), "\r") || strings.Contains(buf.String(), "\t") || strings.Contains(buf.String(), "\x1b") || strings.Contains(buf.String(), "\x00") {
				t.Fatalf("unsafe controls survived: %q", buf.String())
			}
		})
	}
}

func TestWriteAPIErrorHumanAppliesSanitizedFallbacks(t *testing.T) {
	var buf bytes.Buffer
	if !output.WriteAPIErrorHuman(&buf, &api.Error{Code: "\x1b[31m", Message: "\x00"}) {
		t.Fatalf("API WriteAPIErrorHuman() = false")
	}
	if got, want := buf.String(), "Error: api_error\nCode: api_error\nRetryable: false\n"; got != want {
		t.Fatalf("API fallback = %q, want %q", got, want)
	}

	buf.Reset()
	if !output.WriteAPIErrorHuman(&buf, &api.ProtocolError{Detail: "\x1b[31m", RequestID: "\x00"}) {
		t.Fatalf("protocol WriteAPIErrorHuman() = false")
	}
	if got, want := buf.String(), "Error: api protocol error\nCode: api_protocol_error\n"; got != want {
		t.Fatalf("protocol fallback = %q, want %q", got, want)
	}
}

func assertHumanErrorOmitsOperationalContext(t *testing.T, text string) {
	t.Helper()
	for _, rejected := range []string{"Retry after:", "Rate limit:", "Status:"} {
		if strings.Contains(text, rejected) {
			t.Fatalf("human error included rejected context %q:\n%s", rejected, text)
		}
	}
}
