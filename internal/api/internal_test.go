package api

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	date := now.Add(5 * time.Second).Format(http.TimeFormat)
	past := now.Add(-5 * time.Second).Format(http.TimeFormat)
	tests := []struct {
		raw  string
		want *time.Duration
	}{
		{" 2 ", durationPtr(2 * time.Second)},
		{"0", durationPtr(0)},
		{"-1", nil},
		{"1.5", nil},
		{"text", nil},
		{date, durationPtr(5 * time.Second)},
		{past, durationPtr(0)},
		{"9223372036854775808", nil},
	}
	for _, tt := range tests {
		got := parseRetryAfter(tt.raw, now)
		if tt.want == nil {
			if got != nil {
				t.Fatalf("parseRetryAfter(%q) = %v, want nil", tt.raw, *got)
			}
			continue
		}
		if got == nil || *got != *tt.want {
			t.Fatalf("parseRetryAfter(%q) = %v, want %v", tt.raw, got, *tt.want)
		}
	}
}

func TestCaptureResponseMetaParsesHeadersCaseInsensitively(t *testing.T) {
	header := http.Header{
		"x-request-id":               {"req-lower"},
		"x-ratelimit-limit":          {"10"},
		"X-RateLimit-Remaining":      {"bad"},
		"Retry-After":                {"1"},
		"Idempotent-Replayed":        {"TRUE"},
		"x-chab-idempotency-outcome": {"ReLeAsEd"},
	}
	meta := captureResponseMeta(http.StatusOK, header, time.Now())
	if meta.RequestID != "req-lower" || meta.HeaderRequestID != "req-lower" {
		t.Fatalf("request id meta = %#v", meta)
	}
	if meta.RateLimit.Limit == nil || *meta.RateLimit.Limit != 10 || meta.RateLimit.RawLimit != "10" {
		t.Fatalf("rate limit = %#v", meta.RateLimit)
	}
	if meta.RateLimit.Remaining != nil || meta.RateLimit.RawRemaining != "bad" {
		t.Fatalf("remaining = %#v", meta.RateLimit)
	}
	if meta.RetryAfter.Wait == nil || *meta.RetryAfter.Wait != time.Second ||
		meta.IdempotentReplayed == nil || !*meta.IdempotentReplayed {
		t.Fatalf("retry/replay = %#v", meta)
	}
	if meta.IdempotencyOutcome != "released" {
		t.Fatalf("idempotency outcome = %q", meta.IdempotencyOutcome)
	}
}

func TestCaptureResponseMetaPreservesPresentEmptyHeaders(t *testing.T) {
	meta := captureResponseMeta(http.StatusTooManyRequests, http.Header{
		"X-RateLimit-Limit":     {""},
		"X-RateLimit-Remaining": {""},
		"X-RateLimit-Reset":     {""},
		"Retry-After":           {""},
	}, time.Now())
	if !meta.RateLimit.LimitPresent || !meta.RateLimit.RemainingPresent || !meta.RateLimit.ResetPresent ||
		meta.RateLimit.Limit != nil || meta.RateLimit.Remaining != nil || meta.RateLimit.Reset != nil ||
		!meta.RetryAfter.Present || meta.RetryAfter.Wait != nil {
		t.Fatalf("present-empty metadata = %#v", meta)
	}
}

func TestEnvelopeMetadataSanitizationFailsClosed(t *testing.T) {
	env := successEnvelope{
		Meta: map[string]json.RawMessage{
			"broken": json.RawMessage(`{"unterminated":`),
		},
	}
	for _, test := range []struct {
		name  string
		apply func(*ResponseMeta) *ProtocolError
	}{
		{
			name: "bearer",
			apply: func(meta *ResponseMeta) *ProtocolError {
				return (&Client{}).applyEnvelopeMeta(meta, env, http.StatusOK, requestRedactor{})
			},
		},
		{
			name: "bootstrap",
			apply: func(meta *ResponseMeta) *ProtocolError {
				return (&BootstrapClient{}).applyEnvelopeMeta(meta, env, http.StatusOK)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			meta := ResponseMeta{RequestID: "req-safe"}
			protocol := test.apply(&meta)
			if protocol == nil || protocol.Detail != "malformed response metadata" {
				t.Fatalf("protocol = %#v", protocol)
			}
			if meta.RawMeta != nil || protocol.Meta.RawMeta != nil {
				t.Fatalf("metadata was not failed closed: meta=%#v protocol=%#v", meta, protocol)
			}
		})
	}
}

func TestRetryableTransportErrorClassification(t *testing.T) {
	if !isRetryableTransportError(&net.OpError{Op: "dial", Err: errors.New("connection refused")}) {
		t.Fatalf("dial connection failure should be retryable")
	}
	if !isRetryableTransportError(&neturl.Error{Err: testNetError{timeout: true}}) {
		t.Fatalf("url-wrapped timeout should be retryable")
	}
	if isRetryableTransportError(context.Canceled) || isRetryableTransportError(context.DeadlineExceeded) {
		t.Fatalf("context cancellation/deadline should not be retryable")
	}
	if isRetryableTransportError(errors.New("plain")) {
		t.Fatalf("plain error should not be retryable")
	}
	if isRetryableTransportError(x509.UnknownAuthorityError{}) {
		t.Fatalf("certificate failure should not be retryable")
	}
	if isRetryableTransportError(&neturl.Error{Err: x509.UnknownAuthorityError{}}) {
		t.Fatalf("url-wrapped certificate failure should not be retryable")
	}
}

func TestBackoffWaitAndJitterBounds(t *testing.T) {
	wants := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second}
	for i, want := range wants {
		if got := backoffWait(i + 1); got != want {
			t.Fatalf("backoffWait(%d) = %v, want %v", i+1, got, want)
		}
	}

	for i := 0; i < 100; i++ {
		got := randomBackoffJitter(time.Second)
		if got < time.Second || got > 1100*time.Millisecond {
			t.Fatalf("randomBackoffJitter() = %v, want within [1s, 1.1s]", got)
		}
	}
}

func TestRealSleeperPreCanceledContextReturnsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (realSleeper{}).Sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("realSleeper.Sleep() error = %v, want context.Canceled", err)
	}
}

func TestRequestURLStringCases(t *testing.T) {
	base, err := parseBaseURL("https://api.example.test/v1/")
	if err != nil {
		t.Fatalf("parseBaseURL() error = %v", err)
	}
	client := &Client{base: base}
	got := client.requestURL("/projects", nil)
	if got != "https://api.example.test/v1/projects" {
		t.Fatalf("requestURL() = %q", got)
	}
	got = client.requestURL("", nil)
	if got != "https://api.example.test/v1" {
		t.Fatalf("requestURL(empty) = %q", got)
	}
}

func TestNewHTTPClientAndTransportOptions(t *testing.T) {
	const apiKey = "ak_test|secret"
	transport := &recordingRoundTripper{}

	defaultClient, err := New(Options{BaseURL: "https://api.example.test/api/v1", APIKey: apiKey})
	if err != nil {
		t.Fatalf("New(default) error = %v", err)
	}
	if defaultClient.httpClient == nil || defaultClient.httpClient.Timeout != 30*time.Second || defaultClient.httpClient.Transport != nil {
		t.Fatalf("default http client = %#v", defaultClient.httpClient)
	}
	if err := defaultClient.httpClient.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("default CheckRedirect() error = %v, want http.ErrUseLastResponse", err)
	}

	transportClient, err := New(Options{BaseURL: "https://api.example.test/api/v1", APIKey: apiKey, Transport: transport})
	if err != nil {
		t.Fatalf("New(transport) error = %v", err)
	}
	if transportClient.httpClient.Timeout != 30*time.Second || transportClient.httpClient.Transport != transport {
		t.Fatalf("transport http client = %#v", transportClient.httpClient)
	}
	if err := transportClient.httpClient.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("transport CheckRedirect() error = %v, want http.ErrUseLastResponse", err)
	}

	fullHTTPClient := &http.Client{Timeout: 7 * time.Second, Transport: transport}
	ownedClient, err := New(Options{BaseURL: "https://api.example.test/api/v1", APIKey: apiKey, HTTPClient: fullHTTPClient})
	if err != nil {
		t.Fatalf("New(http client) error = %v", err)
	}
	if ownedClient.httpClient == fullHTTPClient || ownedClient.httpClient.Timeout != 7*time.Second || ownedClient.httpClient.Transport != transport {
		t.Fatalf("cloned http client = %#v, want independent clone of %#v", ownedClient.httpClient, fullHTTPClient)
	}
	if err := ownedClient.httpClient.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("cloned CheckRedirect() error = %v, want http.ErrUseLastResponse", err)
	}
	if fullHTTPClient.CheckRedirect != nil {
		t.Fatal("New() mutated the caller-owned HTTP client")
	}

	_, err = New(Options{BaseURL: "https://api.example.test/api/v1", APIKey: apiKey, HTTPClient: fullHTTPClient, Transport: transport})
	var usage *UsageError
	if !errors.As(err, &usage) || usage.ExitCode() != 1 {
		t.Fatalf("New(ambiguous) error = %T %v, want usage error", err, err)
	}
}

func TestNewDisableTransportReuseClonesTransport(t *testing.T) {
	const apiKey = "ak_test|secret"
	defaultClient, err := New(Options{BaseURL: "https://api.example.test/api/v1", APIKey: apiKey, DisableTransportReuse: true})
	if err != nil {
		t.Fatalf("New(default no-reuse) error = %v", err)
	}
	defaultTransport, ok := defaultClient.httpClient.Transport.(*http.Transport)
	if !ok || defaultTransport == nil || !defaultTransport.DisableKeepAlives {
		t.Fatalf("default no-reuse transport = %#v, want *http.Transport with keep-alives disabled", defaultClient.httpClient.Transport)
	}

	suppliedTransport := &http.Transport{}
	transportClient, err := New(Options{BaseURL: "https://api.example.test/api/v1", APIKey: apiKey, Transport: suppliedTransport, DisableTransportReuse: true})
	if err != nil {
		t.Fatalf("New(transport no-reuse) error = %v", err)
	}
	clonedTransport, ok := transportClient.httpClient.Transport.(*http.Transport)
	if !ok || clonedTransport == nil || clonedTransport == suppliedTransport || !clonedTransport.DisableKeepAlives || suppliedTransport.DisableKeepAlives {
		t.Fatalf("transport no-reuse clone = %#v, original = %#v", transportClient.httpClient.Transport, suppliedTransport)
	}

	fullHTTPClient := &http.Client{Timeout: 7 * time.Second, Transport: suppliedTransport}
	ownedClient, err := New(Options{BaseURL: "https://api.example.test/api/v1", APIKey: apiKey, HTTPClient: fullHTTPClient, DisableTransportReuse: true})
	if err != nil {
		t.Fatalf("New(http client no-reuse) error = %v", err)
	}
	ownedTransport, ok := ownedClient.httpClient.Transport.(*http.Transport)
	if !ok || ownedTransport == nil || ownedTransport == suppliedTransport || !ownedTransport.DisableKeepAlives || fullHTTPClient.Transport != suppliedTransport {
		t.Fatalf("http client no-reuse clone = %#v, original = %#v", ownedClient.httpClient.Transport, fullHTTPClient.Transport)
	}

	_, err = New(Options{BaseURL: "https://api.example.test/api/v1", APIKey: apiKey, Transport: &recordingRoundTripper{}, DisableTransportReuse: true})
	var usage *UsageError
	if !errors.As(err, &usage) || usage.Field != "transport" {
		t.Fatalf("New(custom no-reuse transport) error = %T %v, want transport usage error", err, err)
	}
}

func TestDisableTransportReusePreventsHiddenGETReplay(t *testing.T) {
	const apiKey = "ak_test|secret"
	var proofRequests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/warmup":
			writeTestEnvelope(w, http.StatusOK)
		case "/v1/proof":
			atomic.AddInt32(&proofRequests, 1)
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response writer cannot hijack")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatalf("Hijack() error = %v", err)
			}
			_ = conn.Close()
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := New(Options{
		BaseURL:               server.URL + "/v1",
		APIKey:                apiKey,
		MaxAttempts:           1,
		DisableTransportReuse: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var warmup map[string]bool
	if _, err := client.Get(context.Background(), "warmup", nil, &warmup); err != nil {
		t.Fatalf("warmup Get() error = %v", err)
	}
	var proof map[string]bool
	if _, err := client.Get(context.Background(), "proof", nil, &proof); err == nil {
		t.Fatal("proof Get() succeeded, want closed-connection error")
	}
	if got := atomic.LoadInt32(&proofRequests); got != 1 {
		t.Fatalf("proof physical requests = %d, want 1", got)
	}
}

func TestValidateIdempotencyKey(t *testing.T) {
	if err := validateIdempotencyKey("visible-ASCII_123"); err != nil {
		t.Fatalf("validateIdempotencyKey(valid) error = %v", err)
	}
	for _, key := range []string{"", "has space", "bad\nkey", string(rune(0x80))} {
		if err := validateIdempotencyKey(key); err == nil {
			t.Fatalf("validateIdempotencyKey(%q) succeeded", key)
		}
	}
}

type testNetError struct {
	timeout bool
}

func (e testNetError) Error() string   { return "net error" }
func (e testNetError) Timeout() bool   { return e.timeout }
func (e testNetError) Temporary() bool { return e.timeout }

type recordingRoundTripper struct{}

func (*recordingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected request")
}

func writeTestEnvelope(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"data":{"ok":true},"request_id":"req_test"}`))
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}
