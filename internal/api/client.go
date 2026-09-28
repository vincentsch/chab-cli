// Package api contains the shared HTTP runtime for Chab-SaaS public API v1.
package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/redact"
)

// Sleeper waits between retry attempts and honors context cancellation.
type Sleeper interface {
	Sleep(ctx context.Context, d time.Duration) error
}

// Options configures a Client from explicit, already-resolved values. The api
// package itself reads no flags, environment variables, or files. Nil hook
// fields receive production defaults in New.
type Options struct {
	BaseURL          string
	APIKey           string
	Locale           string
	UserAgentVersion string
	HTTPClient       *http.Client
	// Transport customizes the package-owned default client. It is mutually
	// exclusive with HTTPClient because a caller-owned client owns its transport
	// and timeout. New clones caller-owned clients before enforcing the
	// authenticated API client's no-redirect policy.
	Transport     http.RoundTripper
	Sleeper       Sleeper
	Now           func() time.Time
	BackoffJitter func(time.Duration) time.Duration
	// MaxAttempts defaults to the shared retry policy when <= 0. Use 1 with
	// DisableTransportReuse for contracts that require one physical request.
	MaxAttempts int
	// DisableTransportReuse prevents net/http from replaying a request on a
	// previously reused connection after a write failure. Use with MaxAttempts=1
	// for proof or secret-bearing operations that require one wire attempt.
	DisableTransportReuse bool
	DebugWriter           io.Writer
	// SecretValues are per-invocation literals that must be redacted from
	// debug output and rendered API/transport errors. They do not alter
	// successful response bodies.
	SecretValues []string
	AllowNoAuth  bool
}

// OptionsFromRuntime maps runtime resolution, credential lookup, version
// metadata, and the parsed --debug state to API client options.
func OptionsFromRuntime(runtime config.Runtime, credential auth.Credential, userAgentVersion string, debug bool, debugOutput io.Writer) Options {
	opts := Options{
		BaseURL:          runtime.APIBaseURL,
		APIKey:           credential.APIKey,
		Locale:           runtime.Locale,
		UserAgentVersion: userAgentVersion,
		SecretValues:     []string{credential.APIKey},
	}
	if debug && debugOutput != nil {
		opts.DebugWriter = debugOutput
	}
	return opts
}

// Client is safe for concurrent use. All per-request state lives in the
// request loop.
type Client struct {
	base             *url.URL
	apiKey           string
	locale           string
	userAgentVersion string
	httpClient       *http.Client
	sleeper          Sleeper
	now              func() time.Time
	backoffJitter    func(time.Duration) time.Duration
	maxAttempts      int
	debugWriter      io.Writer
	debugMu          *sync.Mutex
	secretReplacer   *strings.Replacer
}

// ExactJSONBody marks an already-encoded JSON value whose bytes must be sent
// unchanged. The shared encoder still validates the value before constructing
// a request.
type ExactJSONBody []byte

// New validates and defaults an API client from explicit options.
func New(opts Options) (*Client, error) {
	base, err := validateBaseURL(opts.BaseURL)
	if err != nil {
		return nil, &UsageError{Field: "base_url", Detail: "must be an absolute http or https URL without userinfo, query, or fragment", Err: err}
	}
	if opts.APIKey == "" && !opts.AllowNoAuth {
		return nil, &UsageError{Field: "api_key", Detail: "must not be empty"}
	}

	userAgentVersion := opts.UserAgentVersion
	if userAgentVersion == "" {
		userAgentVersion = "dev"
	}

	if opts.HTTPClient != nil && opts.Transport != nil {
		return nil, &UsageError{Field: "transport", Detail: "must not be provided with http_client"}
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		// A bare transport keeps timeout ownership in this package while still
		// letting tests and integrations replace the wire behavior.
		transport := opts.Transport
		if opts.DisableTransportReuse {
			transport, err = singleAttemptTransport(transport)
			if err != nil {
				return nil, err
			}
		}
		httpClient = &http.Client{Timeout: 30 * time.Second, Transport: transport}
	} else {
		// Keep caller-owned timeout, transport, jar, and other client settings
		// without mutating the client supplied to New.
		cloned := *httpClient
		if opts.DisableTransportReuse {
			cloned.Transport, err = singleAttemptTransport(cloned.Transport)
			if err != nil {
				return nil, err
			}
		}
		httpClient = &cloned
	}
	// API endpoints are not navigation targets. Following a redirect could
	// replay bearer credentials, idempotency keys, or unsafe request bodies to
	// a response-controlled location.
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	sleeper := opts.Sleeper
	if sleeper == nil {
		sleeper = realSleeper{}
	}

	now := opts.Now
	if now == nil {
		now = time.Now
	}

	backoffJitter := opts.BackoffJitter
	if backoffJitter == nil {
		backoffJitter = randomBackoffJitter
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}

	return &Client{
		base:             base,
		apiKey:           opts.APIKey,
		locale:           opts.Locale,
		userAgentVersion: userAgentVersion,
		httpClient:       httpClient,
		sleeper:          sleeper,
		now:              now,
		backoffJitter:    backoffJitter,
		maxAttempts:      maxAttempts,
		debugWriter:      opts.DebugWriter,
		debugMu:          &sync.Mutex{},
		secretReplacer:   newSecretReplacer(opts.SecretValues),
	}, nil
}

func singleAttemptTransport(transport http.RoundTripper) (http.RoundTripper, error) {
	if transport == nil {
		base, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, &UsageError{Field: "transport", Detail: "default transport cannot be cloned for single-attempt requests"}
		}
		cloned := base.Clone()
		configureSingleAttemptTransport(cloned)
		return cloned, nil
	}
	httpTransport, ok := transport.(*http.Transport)
	if !ok {
		return nil, &UsageError{Field: "transport", Detail: "single-attempt requests require a cloneable *http.Transport"}
	}
	cloned := httpTransport.Clone()
	configureSingleAttemptTransport(cloned)
	return cloned, nil
}

func configureSingleAttemptTransport(cloned *http.Transport) {
	cloned.DisableKeepAlives = true
	cloned.ForceAttemptHTTP2 = false
	cloned.Protocols = new(http.Protocols)
	cloned.Protocols.SetHTTP1(true)
	cloned.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	if cloned.TLSClientConfig != nil {
		cloned.TLSClientConfig = cloned.TLSClientConfig.Clone()
		cloned.TLSClientConfig.NextProtos = []string{"http/1.1"}
	}
}

func (c *Client) singleAttemptClient() (*Client, error) {
	transport, err := singleAttemptTransport(c.httpClient.Transport)
	if err != nil {
		return nil, err
	}
	httpClient := *c.httpClient
	httpClient.Transport = transport
	return &Client{
		base: c.base, apiKey: c.apiKey, locale: c.locale,
		userAgentVersion: c.userAgentVersion, httpClient: &httpClient,
		sleeper: c.sleeper, now: c.now, backoffJitter: c.backoffJitter,
		maxAttempts: 1, debugWriter: c.debugWriter,
		debugMu:        c.debugMu,
		secretReplacer: c.secretReplacer,
	}, nil
}

// Get sends a GET request. It accepts no body and no idempotency option by
// construction.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) (ResponseMeta, error) {
	return c.do(ctx, http.MethodGet, path, query, nil, IdempotencyNone, nil, out)
}

// Post sends a POST request with an optional JSON body.
func (c *Client) Post(ctx context.Context, path string, query url.Values, body any, idem Idempotency, out any) (ResponseMeta, error) {
	return c.do(ctx, http.MethodPost, path, query, body, idem, nil, out)
}

// Put sends a PUT request with an optional JSON body.
func (c *Client) Put(ctx context.Context, path string, query url.Values, body any, idem Idempotency, out any) (ResponseMeta, error) {
	return c.do(ctx, http.MethodPut, path, query, body, idem, nil, out)
}

// Patch sends a PATCH request with an optional JSON body.
func (c *Client) Patch(ctx context.Context, path string, query url.Values, body any, idem Idempotency, out any) (ResponseMeta, error) {
	return c.do(ctx, http.MethodPatch, path, query, body, idem, nil, out)
}

// Delete sends a DELETE request with an optional JSON body.
func (c *Client) Delete(ctx context.Context, path string, query url.Values, body any, idem Idempotency, out any) (ResponseMeta, error) {
	return c.do(ctx, http.MethodDelete, path, query, body, idem, nil, out)
}

// DoWithHeaders sends a typed request with caller-owned headers that are not
// otherwise managed by the shared client. It is intended for narrow protocol
// extensions such as management approval proofs.
func (c *Client) DoWithHeaders(ctx context.Context, method, path string, query url.Values, body any, idem Idempotency, header http.Header, out any) (ResponseMeta, error) {
	return c.do(ctx, method, path, query, body, idem, header, out)
}

// DoRaw runs the shared request lifecycle and decodes the success envelope for
// raw API consumers. Bodyless callers must pass untyped nil for body.
func (c *Client) DoRaw(ctx context.Context, method, path string, query url.Values, body any, idem Idempotency) (RawResult, error) {
	status, responseBody, meta, redactor, err := c.send(ctx, method, path, query, body, idem, nil)
	if err != nil {
		return RawResult{}, err
	}
	return c.decodeRaw(status, responseBody, meta, redactor)
}

// DoRawWithHeaders mirrors DoRaw with caller-owned headers that are not
// otherwise managed by the shared client.
func (c *Client) DoRawWithHeaders(ctx context.Context, method, path string, query url.Values, body any, idem Idempotency, header http.Header) (RawResult, error) {
	status, responseBody, meta, redactor, err := c.send(ctx, method, path, query, body, idem, header)
	if err != nil {
		return RawResult{}, err
	}
	return c.decodeRaw(status, responseBody, meta, redactor)
}

// do owns typed envelope decoding after the shared request lifecycle.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, idem Idempotency, header http.Header, out any) (ResponseMeta, error) {
	status, responseBody, meta, redactor, err := c.send(ctx, method, path, query, body, idem, header)
	if err != nil {
		return ResponseMeta{}, err
	}
	return c.decodeResponse(status, responseBody, meta, out, redactor)
}

// send owns the shared request lifecycle for all HTTP verbs: preflight checks,
// request construction, retries, and metadata capture. Decoding is the caller's
// responsibility.
func (c *Client) send(ctx context.Context, method, path string, query url.Values, body any, idem Idempotency, header http.Header) (int, []byte, ResponseMeta, requestRedactor, error) {
	if idem.singleAttempt {
		one, err := c.singleAttemptClient()
		if err != nil {
			return 0, nil, ResponseMeta{}, requestRedactor{}, err
		}
		idem.singleAttempt = false
		return one.send(ctx, method, path, query, body, idem, header)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	extraHeader, err := validateExtraHeaders(header)
	if err != nil {
		return 0, nil, ResponseMeta{}, requestRedactor{}, err
	}

	// Resolve replay-sensitive values once before the first attempt. Retries
	// must resend the same idempotency key and exact JSON body bytes.
	key, err := resolveRequestIdempotency(method, idem)
	if err != nil {
		return 0, nil, ResponseMeta{}, requestRedactor{}, err
	}
	redactor := c.newRequestRedactor(key)

	bodyBytes, hasBody, err := marshalRequestBody(body)
	if err != nil {
		return 0, nil, ResponseMeta{}, redactor, err
	}

	target := c.requestURL(path, query)
	remaining := sleepBudget
	var waits []time.Duration

	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		// A fresh *http.Request is required for every attempt because request
		// bodies are readers. The buffered body lets unsafe methods be replayed
		// without re-marshaling user input.
		req, err := c.newRequest(ctx, method, target, bodyBytes, hasBody, key, extraHeader)
		if err != nil {
			return 0, nil, ResponseMeta{}, redactor, &UsageError{Field: "path", Detail: "could not build request URL", Err: redactor.redactErr(err)}
		}

		// Debug output reports idempotency mode only. User-supplied keys are
		// request correlation values, but they should not be copied into logs.
		switch {
		case key != "" && idem.mode == idempotencyExplicit:
			c.debugfWith(redactor, "request %s %s attempt=%d idempotency=explicit", method, target, attempt)
		case key != "" && idem.mode == idempotencyAuto:
			c.debugfWith(redactor, "request %s %s attempt=%d idempotency=generated", method, target, attempt)
		default:
			c.debugfWith(redactor, "request %s %s attempt=%d", method, target, attempt)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			c.debugfWith(redactor, "transport error: %s", err)
			outcome := attemptOutcome{err: err}
			decision := c.retryDecision(method, outcome, attempt, remaining)
			if decision.retry {
				if err := c.sleepRetry(ctx, decision, attempt, &remaining, &waits, redactor); err != nil {
					return 0, nil, ResponseMeta{}, redactor, c.transportError(err, attempt, waits, redactor)
				}
				continue
			}
			if decision.refusedReason != "" {
				c.debugfWith(redactor, "retry refused reason=%s", decision.refusedReason)
			}
			return 0, nil, ResponseMeta{}, redactor, c.transportError(err, attempt, waits, redactor)
		}

		meta := captureResponseMeta(resp.StatusCode, resp.Header, c.now())
		c.debugfWith(redactor, "response status=%d request_id=%s attempt=%d", resp.StatusCode, meta.RequestID, attempt)
		c.debugMetaWith(redactor, meta)

		responseBody, readErr := readAndClose(resp.Body)
		if readErr != nil {
			c.debugfWith(redactor, "transport error: %s", readErr)
			// Once headers have arrived, retrying a body-read failure can hide
			// a partial response and may replay an unsafe request, so surface it.
			return 0, nil, ResponseMeta{}, redactor, c.transportError(readErr, attempt, waits, redactor)
		}

		outcome := attemptOutcome{status: resp.StatusCode, retryAfter: meta.RetryAfter, idempotencyOutcome: meta.IdempotencyOutcome}
		decision := c.retryDecision(method, outcome, attempt, remaining)
		if decision.retry {
			if err := c.sleepRetry(ctx, decision, attempt, &remaining, &waits, redactor); err != nil {
				return 0, nil, ResponseMeta{}, redactor, c.transportError(err, attempt, waits, redactor)
			}
			continue
		}
		if decision.refusedReason != "" {
			c.debugfWith(redactor, "retry refused reason=%s", decision.refusedReason)
		}

		meta.Attempts = attempt
		meta.RetryWaits = cloneDurations(waits)
		meta.IdempotencyUsed = key != ""
		return resp.StatusCode, responseBody, meta, redactor, nil
	}

	return 0, nil, ResponseMeta{}, redactor, c.transportError(fmt.Errorf("request loop exhausted without final response"), c.maxAttempts, waits, redactor)
}

// debugMeta logs response metadata that is useful while scripting. It only uses
// response headers that are already non-secret; debugf still applies the normal
// static and per-invocation redaction pass.
func (c *Client) debugMeta(meta ResponseMeta) {
	c.debugMetaWith(c.newRequestRedactor(""), meta)
}

func (c *Client) debugMetaWith(redactor requestRedactor, meta ResponseMeta) {
	if meta.RateLimit.RawLimit != "" || meta.RateLimit.RawRemaining != "" || meta.RateLimit.RawReset != "" {
		c.debugfWith(redactor, "rate-limit limit=%s remaining=%s reset=%s", meta.RateLimit.RawLimit, meta.RateLimit.RawRemaining, meta.RateLimit.RawReset)
	}
	if meta.RetryAfter.Raw != "" {
		c.debugfWith(redactor, "retry-after=%s", meta.RetryAfter.Raw)
	}
}

// newRequest builds one attempt. It attaches only deterministic headers and a
// replayable body reader so retries stay byte-for-byte identical.
func (c *Client) newRequest(ctx context.Context, method, target string, bodyBytes []byte, hasBody bool, idempotencyKey string, extraHeader http.Header) (*http.Request, error) {
	var bodyReader io.Reader
	if hasBody {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, bodyReader)
	if err != nil {
		return nil, err
	}
	if hasBody {
		req.ContentLength = int64(len(bodyBytes))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
		req.Header.Set("Content-Type", "application/json")
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "chab/"+c.userAgentVersion)
	if c.locale != "" {
		req.Header.Set("Accept-Language", c.locale)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	for name, values := range extraHeader {
		req.Header.Del(name)
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	return req, nil
}

func validateExtraHeaders(header http.Header) (http.Header, error) {
	if len(header) == 0 {
		return nil, nil
	}
	out := make(http.Header, len(header))
	for name, values := range header {
		if name == "" {
			return nil, &UsageError{Field: "header", Detail: "header name must not be empty"}
		}
		canonical := http.CanonicalHeaderKey(name)
		switch canonical {
		case "Authorization", "Idempotency-Key", "Content-Type", "Accept", "User-Agent", "Accept-Language":
			return nil, &UsageError{Field: "header", Detail: "header " + canonical + " is managed by the API client"}
		}
		for _, value := range values {
			if !validHeaderValue(value) {
				return nil, &UsageError{Field: "header", Detail: "header " + canonical + " contains a control byte"}
			}
			out.Add(canonical, value)
		}
	}
	return out, nil
}

func validHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		b := value[i]
		if b == '\t' {
			continue
		}
		if b < 0x20 || b == 0x7f {
			return false
		}
	}
	return true
}

// sleepRetry records only completed waits. A canceled sleep returns before the
// wait is counted, matching what actually happened on the wire.
func (c *Client) sleepRetry(ctx context.Context, decision retryDecision, attempt int, remaining *time.Duration, waits *[]time.Duration, redactor requestRedactor) error {
	c.debugfWith(redactor, "retry attempt=%d reason=%s wait=%s", attempt+1, decision.reason, decision.wait)
	if err := c.sleeper.Sleep(ctx, decision.wait); err != nil {
		return err
	}
	*waits = append(*waits, decision.wait)
	*remaining -= decision.wait
	return nil
}

// marshalRequestBody distinguishes "no body" from a JSON null body. Callers
// that want no Content-Type and no bytes must pass untyped nil. ExactJSONBody
// bypasses normal JSON re-encoding but is still validated and copied into the
// request-owned retry buffer.
func marshalRequestBody(body any) ([]byte, bool, error) {
	if body == nil {
		return nil, false, nil
	}
	if exact, ok := body.(ExactJSONBody); ok {
		if !json.Valid(exact) {
			return nil, false, &UsageError{Field: "body", Detail: "could not marshal JSON request body", Err: errors.New("exact JSON body is invalid")}
		}
		return append([]byte(nil), exact...), true, nil
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, false, &UsageError{Field: "body", Detail: "could not marshal JSON request body", Err: err}
	}
	return bodyBytes, true, nil
}

func readAndClose(body io.ReadCloser) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	defer body.Close()
	return io.ReadAll(body)
}

func cloneDurations(values []time.Duration) []time.Duration {
	if values == nil {
		return nil
	}
	return append([]time.Duration(nil), values...)
}

// newSecretReplacer builds an immutable longest-first replacer for raw secrets
// and the encoded forms that may appear in URLs, JSON, or diagnostics.
func newSecretReplacer(values []string) *strings.Replacer {
	seen := map[string]bool{}
	unique := make([]string, 0, len(values))
	add := func(value string) {
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		unique = append(unique, value)
	}
	for _, value := range values {
		for _, form := range redact.ExactForms(value) {
			add(form)
		}
	}
	sort.SliceStable(unique, func(i, j int) bool {
		return len(unique[i]) > len(unique[j])
	})

	// Longer values must win over their prefixes; otherwise "abc" would redact
	// the start of "abcdef" and leave "def" behind.
	pairs := make([]string, 0, len(unique)*2)
	for _, value := range unique {
		pairs = append(pairs, value, "[REDACTED]")
	}
	if len(pairs) == 0 {
		return nil
	}
	return strings.NewReplacer(pairs...)
}

func (c *Client) redactDynamic(s string) string {
	if c == nil || c.secretReplacer == nil {
		return s
	}
	return c.secretReplacer.Replace(s)
}

// redactText applies the two redaction passes in the order used for debug and
// rendered errors: first known secret shapes, then exact values registered for
// this invocation.
func (c *Client) redactText(s string) string {
	return c.redactDynamic(redact.String(s))
}

// requestRedactor applies both long-lived client secrets and one logical
// request's dynamic secret without changing the shared Client.
type requestRedactor struct {
	client   *Client
	replacer *strings.Replacer
}

// newRequestRedactor composes the immutable client redactor with one
// request-only secret, currently the resolved idempotency key. Keeping the
// second replacer outside Client avoids mutating shared state while concurrent
// requests are in flight.
func (c *Client) newRequestRedactor(secret string) requestRedactor {
	var replacer *strings.Replacer
	if secret != "" {
		replacer = newSecretReplacer([]string{secret})
	}
	return requestRedactor{client: c, replacer: replacer}
}

func (r requestRedactor) redactText(s string) string {
	if r.client != nil {
		s = r.client.redactText(s)
	} else {
		s = redact.String(s)
	}
	if r.replacer != nil {
		s = r.replacer.Replace(s)
	}
	return s
}

func (r requestRedactor) redactErr(err error) error {
	if err == nil {
		return nil
	}
	redacted := r.redactText(err.Error())
	if redacted == err.Error() {
		return err
	}
	return &redactedError{msg: redacted, err: err}
}

func (r requestRedactor) redactMeta(meta ResponseMeta) ResponseMeta {
	meta = redactResponseMeta(meta, r.redactText)
	redactor := r
	meta.errorRedactor = &redactor
	return meta
}

// redactedError preserves the original error for errors.Is/errors.As while
// exposing only the redacted text through Error().
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string {
	if e == nil {
		return ""
	}
	return e.msg
}

func (e *redactedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (c *Client) redactErr(err error) error {
	if err == nil || c == nil || c.secretReplacer == nil {
		return err
	}
	// Keep the original error in the unwrap chain so retry and diagnostics code
	// can still use errors.As, while normal rendering sees the redacted message.
	return &redactedError{msg: c.redactText(err.Error()), err: err}
}

func (c *Client) transportError(err error, attempt int, waits []time.Duration, redactor requestRedactor) *TransportError {
	// Build transport errors in one place so dynamic redaction and retry-wait
	// cloning stay consistent across dial, sleep, body-read, and loop failures.
	return &TransportError{Err: redactor.redactErr(err), Attempts: attempt, RetryWaits: cloneDurations(waits)}
}
