package testutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

import (
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/redact"
)

const (
	TestRequestID            = "req-header"
	HeaderRequestID          = "X-Request-Id"
	HeaderRateLimitLimit     = "X-RateLimit-Limit"
	HeaderRateLimitRemaining = "X-RateLimit-Remaining"
	HeaderRateLimitReset     = "X-RateLimit-Reset"
	HeaderRetryAfter         = "Retry-After"
	HeaderIdempotentReplayed = "Idempotent-Replayed"
	HeaderIdempotencyKey     = "Idempotency-Key"
	HeaderAcceptLanguage     = "Accept-Language"
)

// APIServer wraps httptest.Server with contract headers and request capture.
type APIServer struct {
	*httptest.Server

	mu       sync.Mutex
	requests []recordedRequest
}

// recordedRequest keeps raw request material private to APIServer so targeted
// assertions can compare exact values while general snapshots stay safe to log.
type recordedRequest struct {
	safe              CapturedRequest
	rawAuthorization  string
	rawIdempotencyKey string
	body              []byte
}

// CapturedRequest is one recorded request with secret-bearing headers and body
// content redacted or summarized. Query values remain raw for exact request
// shape assertions and should not be dumped wholesale in failure messages.
type CapturedRequest struct {
	Method     string
	Path       string
	RequestURI string
	RawQuery   string
	Query      url.Values
	Header     http.Header
	BodyLen    int
	BodySHA256 string
}

// NewAPIServer records every request, stamps contract JSON headers, and
// delegates to handler. The server closes via t.Cleanup.
func NewAPIServer(t *testing.T, handler http.HandlerFunc) *APIServer {
	t.Helper()
	s := &APIServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readRequestBody(t, r)
		s.record(r, body)
		r.Body = io.NopCloser(bytes.NewReader(body))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(HeaderRequestID, TestRequestID)
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func readRequestBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	if r.Body == nil {
		return nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("mock API could not read request body: %v", err)
		return nil
	}
	return body
}

func (s *APIServer) record(r *http.Request, body []byte) {
	sum := sha256.Sum256(body)
	// Keep raw auth/idempotency/body fields out of CapturedRequest. Callers get
	// redacted headers and body metadata by default, then opt into exact checks
	// through assertion helpers that control their own failure messages.
	rec := recordedRequest{
		rawAuthorization:  r.Header.Get("Authorization"),
		rawIdempotencyKey: r.Header.Get(HeaderIdempotencyKey),
		body:              append([]byte(nil), body...),
	}
	rec.safe = CapturedRequest{
		Method:     r.Method,
		Path:       r.URL.EscapedPath(),
		RequestURI: r.RequestURI,
		RawQuery:   r.URL.RawQuery,
		Query:      r.URL.Query(),
		Header:     secretSafeHeader(r.Header),
		BodyLen:    len(body),
		BodySHA256: hex.EncodeToString(sum[:]),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, rec)
}

// secretSafeHeader mirrors request headers but removes values that commonly
// carry credentials before a test can dump CapturedRequest.
func secretSafeHeader(in http.Header) http.Header {
	out := make(http.Header, len(in))
	for key, values := range in {
		copied := make([]string, len(values))
		for i, value := range values {
			if strings.EqualFold(key, "Authorization") || strings.EqualFold(key, HeaderIdempotencyKey) {
				copied[i] = redact.Header(key, value)
				if strings.EqualFold(key, HeaderIdempotencyKey) && value != "" {
					copied[i] = "[REDACTED]"
				}
				continue
			}
			copied[i] = redact.String(value)
		}
		out[key] = copied
	}
	return out
}

// APIBaseURL returns the server URL rooted at the product API prefix.
func (s *APIServer) APIBaseURL() string {
	return s.URL + "/v1"
}

// Count returns the number of requests captured so far.
func (s *APIServer) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

// Requests returns cloned, redacted request snapshots.
func (s *APIServer) Requests() []CapturedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]CapturedRequest, len(s.requests))
	for i, req := range s.requests {
		out[i] = cloneCapturedRequest(req.safe)
	}
	return out
}

// cloneCapturedRequest prevents callers from mutating stored header/query
// snapshots after they inspect a request.
func cloneCapturedRequest(in CapturedRequest) CapturedRequest {
	out := in
	out.Query = cloneValues(in.Query)
	out.Header = in.Header.Clone()
	return out
}

func cloneValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}

// Request returns one cloned, redacted request snapshot.
func (s *APIServer) Request(t *testing.T, i int) CapturedRequest {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if i < 0 || i >= len(s.requests) {
		t.Fatalf("request index %d out of range; count=%d", i, len(s.requests))
	}
	return cloneCapturedRequest(s.requests[i].safe)
}

// AssertNoRequests checks local-only paths without dumping captured requests,
// because raw query values are intentionally available on CapturedRequest.
func (s *APIServer) AssertNoRequests(t *testing.T) {
	t.Helper()
	if count := s.Count(); count != 0 {
		t.Fatalf("request count = %d, want 0", count)
	}
}

// AssertPathSegments compares one request's decoded path segments without
// including raw segment values in mismatch diagnostics.
func (s *APIServer) AssertPathSegments(t *testing.T, i int, want []string) {
	t.Helper()
	req := s.recorded(t, i)
	got, err := decodedPathSegments(req.safe.Path)
	if err != nil {
		t.Fatalf("request %d path could not be decoded: %s", i, valueFingerprint(req.safe.Path))
	}
	if diagnostic := compareSecretValues("path segment", got, want); diagnostic != "" {
		t.Fatalf("request %d %s", i, diagnostic)
	}
}

// AssertQueryValues compares a query key's exact decoded values. A nil want
// means the key must be absent; a non-nil slice means it must be present with
// exactly those values.
func (s *APIServer) AssertQueryValues(t *testing.T, i int, key string, want []string) {
	t.Helper()
	req := s.recorded(t, i)
	got, present := req.safe.Query[key]
	wantPresent := want != nil
	label := redact.String(key)
	if present != wantPresent {
		t.Fatalf("request %d query key %q presence = %t, want %t", i, label, present, wantPresent)
	}
	if !present {
		return
	}
	if diagnostic := compareSecretValues("query value", got, want); diagnostic != "" {
		t.Fatalf("request %d query key %q %s", i, label, diagnostic)
	}
}

func decodedPathSegments(escapedPath string) ([]string, error) {
	if escapedPath == "" || escapedPath == "/" {
		return []string{}, nil
	}
	trimmed := strings.TrimPrefix(escapedPath, "/")
	parts := strings.Split(trimmed, "/")
	out := make([]string, len(parts))
	for index, part := range parts {
		decoded, err := url.PathUnescape(part)
		if err != nil {
			return nil, err
		}
		out[index] = decoded
	}
	return out, nil
}

func compareSecretValues(label string, got, want []string) string {
	if len(got) != len(want) {
		return fmt.Sprintf("%s count = %d, want %d", label, len(got), len(want))
	}
	for index := range got {
		if got[index] != want[index] {
			return fmt.Sprintf("%s %d mismatch: got %s; want %s", label, index, valueFingerprint(got[index]), valueFingerprint(want[index]))
		}
	}
	return ""
}

func valueFingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("length=%d sha256=%s", len(value), hex.EncodeToString(sum[:]))
}

// AssertBearer asserts every captured request sent Authorization: Bearer <key>.
func (s *APIServer) AssertBearer(t *testing.T, key string) {
	t.Helper()
	expected := "Bearer " + key
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		t.Fatalf("request count = 0, want at least 1 request with Authorization: Bearer [REDACTED]")
	}
	for i, req := range s.requests {
		if req.rawAuthorization != expected {
			t.Fatalf("request %d Authorization = %q, want %q", i, redactAuthorization(req.rawAuthorization, key), redactAuthorization(expected, key))
		}
	}
}

// AssertIdempotencyKey compares one request's exact raw idempotency key while
// keeping both got and want redacted in the failure message.
func (s *APIServer) AssertIdempotencyKey(t *testing.T, i int, key string) {
	t.Helper()
	req := s.recorded(t, i)
	if req.rawIdempotencyKey != key {
		t.Fatalf("request %d Idempotency-Key = %s, want %s", i, redactedSecretLabel(req.rawIdempotencyKey), redactedSecretLabel(key))
	}
}

// AssertNoIdempotencyKey requires that a request did not send an
// Idempotency-Key header.
func (s *APIServer) AssertNoIdempotencyKey(t *testing.T, i int) {
	t.Helper()
	req := s.recorded(t, i)
	if req.rawIdempotencyKey != "" {
		t.Fatalf("request %d Idempotency-Key = %s, want absent", i, redactedSecretLabel(req.rawIdempotencyKey))
	}
}

// AssertIdempotencyKeyMatches checks generated key shape without printing the
// generated value on failure.
func (s *APIServer) AssertIdempotencyKeyMatches(t *testing.T, i int, pattern *regexp.Regexp) {
	t.Helper()
	req := s.recorded(t, i)
	if !pattern.MatchString(req.rawIdempotencyKey) {
		t.Fatalf("request %d Idempotency-Key does not match %s: %s", i, pattern, redactedSecretLabel(req.rawIdempotencyKey))
	}
}

// AssertNoIdempotencyKeyLeak asserts one captured request's idempotency key is
// absent from haystack, including escaped forms, without exporting the raw key.
func (s *APIServer) AssertNoIdempotencyKeyLeak(t *testing.T, i int, haystack string) {
	t.Helper()
	req := s.recorded(t, i)
	for _, form := range secretValueForms(req.rawIdempotencyKey) {
		if strings.Contains(haystack, form) {
			t.Fatalf("request %d Idempotency-Key leaked as [REDACTED] in %q", i, redactWithSecrets(haystack, req.rawIdempotencyKey))
		}
	}
}

// AssertNoBody requires that the captured request had no body bytes.
func (s *APIServer) AssertNoBody(t *testing.T, i int) {
	t.Helper()
	req := s.recorded(t, i)
	if len(req.body) != 0 {
		t.Fatalf("request %d body length = %d, want 0; sha256=%s", i, len(req.body), req.safe.BodySHA256)
	}
}

// DecodeJSONBody decodes a captured request body and redacts caller-supplied
// secret values if parsing fails and the body must be shown.
func (s *APIServer) DecodeJSONBody(t *testing.T, i int, dst any, secretValues ...string) {
	t.Helper()
	req := s.recorded(t, i)
	if err := json.Unmarshal(req.body, dst); err != nil {
		t.Fatalf("request %d body JSON parse error = %v; body=%s", i, err, redactWithSecrets(string(req.body), secretValues...))
	}
}

// BodyString returns the raw captured body for tests that need exact shape or
// numeric-preservation assertions. Prefer DecodeJSONBody when parser failures
// may need to print a secret-bearing body.
func (s *APIServer) BodyString(t *testing.T, i int) string {
	t.Helper()
	req := s.recorded(t, i)
	return string(req.body)
}

// recorded returns a defensive copy of one full request, including private raw
// fields for assertion helpers.
func (s *APIServer) recorded(t *testing.T, i int) recordedRequest {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if i < 0 || i >= len(s.requests) {
		t.Fatalf("request index %d out of range; count=%d", i, len(s.requests))
	}
	req := s.requests[i]
	req.safe = cloneCapturedRequest(req.safe)
	req.body = append([]byte(nil), req.body...)
	return req
}

// redactWithSecrets replaces caller-marked values in raw, URL-escaped, and
// JSON-escaped forms before applying the general project redactor.
func redactWithSecrets(text string, secretValues ...string) string {
	pairs := make([]string, 0, len(secretValues)*6)
	for _, value := range secretValues {
		for _, form := range secretValueForms(value) {
			pairs = append(pairs, form, "[REDACTED]")
		}
	}
	if len(pairs) > 0 {
		text = strings.NewReplacer(pairs...).Replace(text)
	}
	return redact.String(text)
}

// redactAuthorization redacts both the expected key and any Bearer token that
// actually appeared in the request.
func redactAuthorization(raw, expectedKey string) string {
	secrets := []string{expectedKey}
	if strings.HasPrefix(raw, "Bearer ") {
		secrets = append(secrets, strings.TrimPrefix(raw, "Bearer "))
	}
	return redactWithSecrets(raw, secrets...)
}

func redactedSecretLabel(value string) string {
	if value == "" {
		return "<empty>"
	}
	return "[REDACTED]"
}

// secretValueForms enumerates raw, URL-escaped, double-escaped, and JSON string
// forms most likely to appear in CLI output or transport diagnostics.
func secretValueForms(value string) []string {
	if value == "" {
		return nil
	}
	forms := make([]string, 0, 12)
	seen := make(map[string]struct{}, cap(forms))
	add := func(form string) {
		if form == "" {
			return
		}
		if _, exists := seen[form]; exists {
			return
		}
		seen[form] = struct{}{}
		forms = append(forms, form)
	}
	addWithDelimiterCase := func(form string) {
		add(form)
		add(strings.NewReplacer("%257C", "%257c", "%7C", "%7c").Replace(form))
	}

	add(value)
	for _, encoded := range []string{url.QueryEscape(value), url.PathEscape(value)} {
		addWithDelimiterCase(encoded)
		addWithDelimiterCase(url.QueryEscape(encoded))
		addWithDelimiterCase(url.PathEscape(encoded))
	}
	if encoded, err := json.Marshal(value); err == nil && len(encoded) >= 2 {
		add(string(encoded))
		add(string(encoded[1 : len(encoded)-1]))
	}
	return forms
}

// Envelope builds the documented success envelope shape.
func Envelope(data any) map[string]any {
	return map[string]any{"data": data}
}

// ListEnvelope builds the documented list envelope shape.
func ListEnvelope(data any, pg *api.Pagination) map[string]any {
	env := Envelope(data)
	if pg != nil {
		env["meta"] = map[string]any{"pagination": pg}
	}
	return env
}

// ErrorEnvelope builds the documented error envelope shape.
func ErrorEnvelope(code, message, requestID string, details map[string][]string) map[string]any {
	retryable := code == "rate_limited" || code == "internal_error" || code == "temporarily_unavailable"
	return map[string]any{
		"error": map[string]any{
			"code":      code,
			"message":   message,
			"retryable": retryable,
			"details":   details,
		},
		"request_id": requestID,
	}
}

// WriteJSON writes one newline-terminated JSON response, reporting fixture
// failures through t.Errorf and a deterministic internal_error body.
func WriteJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Errorf("mock API JSON marshal error = %v", err)
		writeInternalError(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(append(data, '\n')); err != nil {
		t.Errorf("mock API response write error = %v", err)
	}
}

// JSONResponse returns a handler that always writes one JSON value.
func JSONResponse(t *testing.T, status int, value any) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(t, w, status, value)
	}
}

func writeInternalError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(`{"error":{"code":"internal_error","message":"mock API fixture failure","details":null},"request_id":"` + TestRequestID + `"}` + "\n"))
}

// RateLimitHeaders sets documented rate-limit and retry headers.
type RateLimitHeaders struct {
	Limit      string
	Remaining  string
	Reset      string
	RetryAfter string
}

// Set writes only the configured headers.
func (h RateLimitHeaders) Set(w http.ResponseWriter) {
	if h.Limit != "" {
		w.Header().Set(HeaderRateLimitLimit, h.Limit)
	}
	if h.Remaining != "" {
		w.Header().Set(HeaderRateLimitRemaining, h.Remaining)
	}
	if h.Reset != "" {
		w.Header().Set(HeaderRateLimitReset, h.Reset)
	}
	if h.RetryAfter != "" {
		w.Header().Set(HeaderRetryAfter, h.RetryAfter)
	}
}

// SequenceHandler serves one handler per request in order.
func SequenceHandler(t *testing.T, handlers ...http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	var mu sync.Mutex
	next := 0
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		i := next
		next++
		mu.Unlock()
		if i >= len(handlers) {
			t.Errorf("mock API sequence exhausted at request %d", i)
			writeInternalError(w)
			return
		}
		handlers[i](w, r)
	}
}

// FailOnContact fails the test on any request and answers internal_error 500.
func FailOnContact(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("mock API received unexpected request: %s %s", r.Method, safeRequestPath(r))
		writeInternalError(w)
	}
}

// safeRequestPath omits the raw query string so unexpected-contact failures do
// not print user-supplied query secrets.
func safeRequestPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return "<unknown>"
	}
	path := r.URL.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

// PageCall is the pagination query one request carried.
type PageCall struct {
	Page       int
	PerPage    int
	PageRaw    string
	PerPageRaw string
	PageSet    bool
	PerPageSet bool
}

// PageCalls extracts pagination query state from captured request snapshots.
func PageCalls(requests []CapturedRequest) []PageCall {
	out := make([]PageCall, 0, len(requests))
	for _, req := range requests {
		out = append(out, pageCall(req.Query))
	}
	return out
}

func pageCall(q url.Values) PageCall {
	call := PageCall{
		Page:       1,
		PageSet:    hasQueryKey(q, "page"),
		PerPageSet: hasQueryKey(q, "per_page"),
	}
	if call.PageSet {
		call.PageRaw = firstQueryValue(q, "page")
		call.Page = parseIntDefault(call.PageRaw, 1)
	}
	if call.PerPageSet {
		call.PerPageRaw = firstQueryValue(q, "per_page")
		call.PerPage = parseIntDefault(call.PerPageRaw, 0)
	}
	return call
}

func hasQueryKey(q url.Values, key string) bool {
	_, ok := q[key]
	return ok
}

func firstQueryValue(q url.Values, key string) string {
	values := q[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func parseIntDefault(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

// PagedListConfig describes a deterministic paginated list fixture.
type PagedListConfig struct {
	Path           string
	Rows           []any
	DefaultPerPage int
	OmitPagination bool
}

// NewPagedListServer serves deterministic pages of cfg.Rows with populated
// pagination metadata unless OmitPagination is set.
func NewPagedListServer(t *testing.T, cfg PagedListConfig) *APIServer {
	t.Helper()
	if cfg.DefaultPerPage <= 0 {
		cfg.DefaultPerPage = 25
	}
	return NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.EscapedPath(); got != cfg.Path {
			t.Errorf("request path = %q, want %q", got, cfg.Path)
			writeInternalError(w)
			return
		}
		call := pageCall(r.URL.Query())
		page := call.Page
		if page < 1 {
			page = 1
		}
		perPage := call.PerPage
		if perPage < 1 {
			perPage = cfg.DefaultPerPage
		}

		start := (page - 1) * perPage
		end := start + perPage
		if start > len(cfg.Rows) {
			start = len(cfg.Rows)
		}
		if end > len(cfg.Rows) {
			end = len(cfg.Rows)
		}
		rows := make([]any, 0, end-start)
		rows = append(rows, cfg.Rows[start:end]...)
		if cfg.OmitPagination {
			WriteJSON(t, w, http.StatusOK, Envelope(rows))
			return
		}
		WriteJSON(t, w, http.StatusOK, ListEnvelope(rows, pagination(page, perPage, len(cfg.Rows), start, end)))
	})
}

// pagination returns the fully populated pagination object expected by current
// list command helpers, including nil from/to pointers on empty pages.
func pagination(page, perPage, total, start, end int) *api.Pagination {
	lastPage := int(math.Ceil(float64(total) / float64(perPage)))
	if lastPage < 1 {
		lastPage = 1
	}
	var from, to *int
	if start < end {
		f := start + 1
		t := end
		from = &f
		to = &t
	}
	return &api.Pagination{
		CurrentPage: page,
		PerPage:     perPage,
		Total:       total,
		LastPage:    lastPage,
		From:        from,
		To:          to,
		HasMore:     page < lastPage,
	}
}

// String is for exact request-shape diagnostics and includes RawQuery. Avoid
// using it when query values may contain secrets.
func (r CapturedRequest) String() string {
	return fmt.Sprintf("%s %s?%s", r.Method, r.Path, r.RawQuery)
}
