package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestFingerprintConfigurationModes(t *testing.T) {
	absent, err := loadFingerprintExpectations(func(string) (string, bool) { return "", false })
	if err != nil || absent.enabled {
		t.Fatalf("absent configuration = %#v, %v", absent, err)
	}

	api := sha256.Sum256([]byte("api-secret"))
	idempotency := sha256.Sum256([]byte("idempotency-secret"))
	validValues := map[string]string{
		expectedAPIKeyEnv:         strings.ToUpper(hex.EncodeToString(api[:])),
		expectedIdempotencyKeyEnv: hex.EncodeToString(idempotency[:]),
	}
	valid, err := loadFingerprintExpectations(mapLookup(validValues))
	if err != nil {
		t.Fatal(err)
	}
	if !valid.enabled || valid.apiKey != api || valid.idempotencyKey != idempotency {
		t.Fatalf("valid configuration = %#v", valid)
	}

	tests := []map[string]string{
		{expectedAPIKeyEnv: hex.EncodeToString(api[:])},
		{expectedAPIKeyEnv: "", expectedIdempotencyKeyEnv: hex.EncodeToString(idempotency[:])},
		{expectedAPIKeyEnv: "not-hex", expectedIdempotencyKeyEnv: hex.EncodeToString(idempotency[:])},
		{expectedAPIKeyEnv: strings.Repeat("a", 62), expectedIdempotencyKeyEnv: hex.EncodeToString(idempotency[:])},
	}
	for index, values := range tests {
		if _, err := loadFingerprintExpectations(mapLookup(values)); err == nil {
			t.Fatalf("invalid configuration %d succeeded", index)
		} else {
			for _, value := range values {
				if value != "" && strings.Contains(err.Error(), value) {
					t.Fatalf("configuration error leaked input: %v", err)
				}
			}
		}
	}
}

func TestMalformedFingerprintConfigurationFailsBeforeListen(t *testing.T) {
	var stdout bytes.Buffer
	values := map[string]string{
		expectedAPIKeyEnv:         "malformed",
		expectedIdempotencyKeyEnv: strings.Repeat("a", 64),
	}
	// The address is intentionally invalid. Receiving the configuration error
	// proves validation returned before serveConfigured reached net.Listen.
	err := serveConfigured("invalid-listen-address", &stdout, log.New(io.Discard, "", 0), mapLookup(values))
	if err == nil || err.Error() != "invalid fingerprint configuration" {
		t.Fatalf("serveConfigured error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("startup protocol was written before configuration validation: %q", stdout.String())
	}
}

func TestFingerprintEnforcementRejectsMismatchesBeforeRouting(t *testing.T) {
	apiKey := `runner-api-key`
	idempotencyKey := `runner-idempotency-key"`
	expectations := fingerprintExpectations{
		enabled:        true,
		apiKey:         sha256.Sum256([]byte(apiKey)),
		idempotencyKey: sha256.Sum256([]byte(idempotencyKey)),
	}

	srv := newServer(log.New(io.Discard, "", 0), expectations)
	matchingRead := apiRequestWithHeaders(srv, http.MethodGet, "/v1/projects", nil, map[string]string{
		"Authorization": "Bearer " + apiKey,
	})
	if matchingRead.Code != http.StatusOK {
		t.Fatalf("matching read status=%d body=%s", matchingRead.Code, matchingRead.Body.String())
	}

	srv = newServer(log.New(io.Discard, "", 0), expectations)
	rejectedAuth := apiRequestWithHeaders(srv, http.MethodGet, "/v1/projects", nil, map[string]string{
		"Authorization": "Bearer substituted",
	})
	if rejectedAuth.Code != http.StatusUnauthorized ||
		rejectedAuth.Body.String() != `{"error":{"code":"invalid_api_token","message":"Credential rejected.","retryable":false},"request_id":"mock-req-000001"}`+"\n" ||
		rejectedAuth.Header().Get("X-Request-Id") != "mock-req-000001" {
		t.Fatalf("auth rejection status=%d headers=%v body=%s", rejectedAuth.Code, rejectedAuth.Header(), rejectedAuth.Body.String())
	}

	srv = newServer(log.New(io.Discard, "", 0), expectations)
	rejectedIdempotency := apiRequestWithHeaders(srv, http.MethodPost, "/v1/projects", strings.NewReader(`{"name":"Rejected"}`), map[string]string{
		"Authorization":   "Bearer " + apiKey,
		"Idempotency-Key": "substituted",
	})
	if rejectedIdempotency.Code != http.StatusConflict ||
		rejectedIdempotency.Body.String() != `{"error":{"code":"idempotency_key_conflict","message":"Idempotency key rejected.","retryable":false},"request_id":"mock-req-000001"}`+"\n" ||
		rejectedIdempotency.Header().Get("X-Request-Id") != "mock-req-000001" {
		t.Fatalf("idempotency rejection status=%d headers=%v body=%s", rejectedIdempotency.Code, rejectedIdempotency.Header(), rejectedIdempotency.Body.String())
	}
	if len(srv.idem) != 0 || len(srv.projects) != 3 {
		t.Fatalf("rejected mutation changed state: replays=%d projects=%d", len(srv.idem), len(srv.projects))
	}
	counter := httptest.NewRecorder()
	srv.ServeHTTP(counter, httptest.NewRequest(http.MethodGet, testURL("/__mock/requests"), nil))
	var count struct {
		APIRequests int `json:"api_requests"`
	}
	decodeBody(t, counter, &count)
	if count.APIRequests != 1 {
		t.Fatalf("rejected request count = %d, want 1", count.APIRequests)
	}

	first := apiRequestWithHeaders(srv, http.MethodPost, "/v1/projects", strings.NewReader(`{"name":"Accepted"}`), map[string]string{
		"Authorization":   "Bearer " + apiKey,
		"Idempotency-Key": idempotencyKey,
	})
	second := apiRequestWithHeaders(srv, http.MethodPost, "/v1/projects", strings.NewReader(`{"name":"Accepted"}`), map[string]string{
		"Authorization":   "Bearer " + apiKey,
		"Idempotency-Key": idempotencyKey,
	})
	if first.Code != http.StatusCreated || second.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("matching idempotency first=%d second headers=%v", first.Code, second.Header())
	}
}

func TestFingerprintEnforcementLogsOnlyReceivedShortHashes(t *testing.T) {
	apiKey := "runner-api-key"
	idempotencyKey := `runner-idempotency-key"`
	apiFingerprint := sha256.Sum256([]byte(apiKey))
	idempotencyFingerprint := sha256.Sum256([]byte(idempotencyKey))
	var logs bytes.Buffer
	srv := newServer(log.New(&logs, "", 0), fingerprintExpectations{
		enabled:        true,
		apiKey:         apiFingerprint,
		idempotencyKey: idempotencyFingerprint,
	})
	_ = apiRequestWithHeaders(srv, http.MethodPost, "/v1/projects", strings.NewReader(`{"name":"Accepted"}`), map[string]string{
		"Authorization":   "Bearer " + apiKey,
		"Idempotency-Key": idempotencyKey,
	})
	got := logs.String()
	for _, forbidden := range []string{
		apiKey,
		idempotencyKey,
		hex.EncodeToString(apiFingerprint[:]),
		hex.EncodeToString(idempotencyFingerprint[:]),
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("log leaked protected value in %q", got)
		}
	}
	if !regexp.MustCompile(`auth=[0-9a-f]{8}`).MatchString(got) || !regexp.MustCompile(`idem=[0-9a-f]{8}`).MatchString(got) {
		t.Fatalf("log missing received short hashes: %q", got)
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func TestSuccessEnvelopeAndHeaders(t *testing.T) {
	srv := newServer(log.New(io.Discard, "", 0))
	rec := apiRequest(srv, http.MethodGet, "/v1/projects", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	decodeBody(t, rec, &body)
	if _, ok := body["data"]; !ok {
		t.Fatalf("body missing data: %#v", body)
	}
	meta, ok := body["meta"].(map[string]any)
	if !ok || meta["request_id"] != rec.Header().Get("X-Request-Id") {
		t.Fatalf("request id body/header mismatch: %#v headers=%v", body, rec.Header())
	}
	for _, name := range []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"} {
		if rec.Header().Get(name) == "" {
			t.Fatalf("missing %s", name)
		}
	}
}

func TestPaginationAndFilters(t *testing.T) {
	srv := newServer(log.New(io.Discard, "", 0))

	rec := apiRequest(srv, http.MethodGet, "/v1/projects?limit=2", nil, true)
	var list struct {
		Data []project `json:"data"`
		Meta struct {
			RequestID  string  `json:"request_id"`
			NextCursor *string `json:"next_cursor"`
			PrevCursor *string `json:"prev_cursor"`
			HasMore    bool    `json:"has_more"`
			Limit      int     `json:"limit"`
		} `json:"meta"`
	}
	decodeBody(t, rec, &list)
	if len(list.Data) != 2 || list.Meta.RequestID == "" || !list.Meta.HasMore || list.Meta.NextCursor == nil || list.Meta.PrevCursor != nil || list.Meta.Limit != 2 {
		t.Fatalf("cursor page = data:%#v meta:%#v", list.Data, list.Meta)
	}
	rec = apiRequest(srv, http.MethodGet, "/v1/projects?limit=2&cursor="+*list.Meta.NextCursor, nil, true)
	var empty struct {
		Data []project `json:"data"`
		Meta struct {
			NextCursor *string `json:"next_cursor"`
			PrevCursor *string `json:"prev_cursor"`
			HasMore    bool    `json:"has_more"`
			Limit      int     `json:"limit"`
		} `json:"meta"`
	}
	decodeBody(t, rec, &empty)
	if len(empty.Data) != 1 || empty.Meta.NextCursor != nil || empty.Meta.PrevCursor == nil || empty.Meta.HasMore || empty.Meta.Limit != 2 {
		t.Fatalf("second cursor page = data:%#v meta:%#v", empty.Data, empty.Meta)
	}

	rec = apiRequest(srv, http.MethodGet, "/v1/projects?status=active&q=dem", nil, true)
	var projects struct {
		Data []project `json:"data"`
	}
	decodeBody(t, rec, &projects)
	if len(projects.Data) != 1 || projects.Data[0].ID != projectOneID {
		t.Fatalf("filtered projects = %#v", projects.Data)
	}
}

func TestTransactionsUseChabCursorShape(t *testing.T) {
	srv := newServer(log.New(io.Discard, "", 0))
	rec := apiRequest(srv, http.MethodGet, "/v1/credits/transactions?limit=2", nil, true)
	var window struct {
		Data []transaction `json:"data"`
		Meta struct {
			RequestID  string  `json:"request_id"`
			NextCursor *string `json:"next_cursor"`
			PrevCursor *string `json:"prev_cursor"`
			HasMore    bool    `json:"has_more"`
			Limit      int     `json:"limit"`
		} `json:"meta"`
	}
	decodeBody(t, rec, &window)
	if len(window.Data) != 2 || window.Data[0].Kind != "adjustment" || window.Data[1].Kind != "debt" {
		t.Fatalf("transactions = %#v", window.Data)
	}
	if window.Meta.RequestID == "" || window.Meta.Limit != 2 || !window.Meta.HasMore || window.Meta.NextCursor == nil || window.Meta.PrevCursor != nil {
		t.Fatalf("cursor meta = %#v", window.Meta)
	}
}

func TestProjectWrites(t *testing.T) {
	srv := newServer(log.New(io.Discard, "", 0))
	rec := apiRequest(srv, http.MethodPost, "/v1/projects", strings.NewReader(`{"status":"active"}`), true)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "The name field is required.") {
		t.Fatalf("validation body = %s", rec.Body.String())
	}

	rec = apiRequest(srv, http.MethodPost, "/v1/projects", strings.NewReader(`{"name":"Created","status":"paused"}`), true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data project `json:"data"`
	}
	decodeBody(t, rec, &created)
	if created.Data.Name != "Created" || created.Data.ID == "" {
		t.Fatalf("created = %#v", created.Data)
	}

	rec = apiRequest(srv, http.MethodPatch, "/v1/projects/"+created.Data.ID, strings.NewReader(`{"status":"active","limit":44}`), true)
	var patched struct {
		Data project `json:"data"`
	}
	decodeBody(t, rec, &patched)
	if patched.Data.Status != "active" || patched.Data.Limit != 44 {
		t.Fatalf("patched = %#v", patched.Data)
	}

	rec = apiRequest(srv, http.MethodDelete, "/v1/projects/"+created.Data.ID, nil, true)
	var deleted struct {
		Data struct {
			ID      string `json:"id"`
			Deleted bool   `json:"deleted"`
		} `json:"data"`
	}
	decodeBody(t, rec, &deleted)
	if deleted.Data.ID != created.Data.ID || !deleted.Data.Deleted {
		t.Fatalf("deleted = %#v", deleted.Data)
	}

	rec = apiRequest(srv, http.MethodGet, "/v1/projects/"+created.Data.ID, nil, true)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "not_found") {
		t.Fatalf("show after delete status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIdempotencyReplay(t *testing.T) {
	srv := newServer(log.New(io.Discard, "", 0))
	first := apiRequestWithHeaders(srv, http.MethodPost, "/v1/projects", strings.NewReader(`{"name":"Replay"}`), map[string]string{"Authorization": "Bearer test-key", "Idempotency-Key": "idem-one"})
	second := apiRequestWithHeaders(srv, http.MethodPost, "/v1/projects", strings.NewReader(`{"name":"Replay"}`), map[string]string{"Authorization": "Bearer test-key", "Idempotency-Key": "idem-one"})
	third := apiRequestWithHeaders(srv, http.MethodPost, "/v1/projects", strings.NewReader(`{"name":"Replay"}`), map[string]string{"Authorization": "Bearer test-key", "Idempotency-Key": "idem-two"})

	if first.Body.String() != second.Body.String() {
		t.Fatalf("replay body changed\nfirst=%s\nsecond=%s", first.Body.String(), second.Body.String())
	}
	if second.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay header = %q", second.Header().Get("Idempotent-Replayed"))
	}
	if third.Body.String() == first.Body.String() || third.Header().Get("Idempotent-Replayed") != "" {
		t.Fatalf("different key did not create distinct response: body=%s header=%q", third.Body.String(), third.Header().Get("Idempotent-Replayed"))
	}

	rec := apiRequest(srv, http.MethodGet, "/v1/projects", nil, true)
	var list struct {
		Data []project `json:"data"`
	}
	decodeBody(t, rec, &list)
	if len(list.Data) != 5 {
		t.Fatalf("project count = %d, want fixtures plus two creates", len(list.Data))
	}
}

func TestAuthAndErrorRoutes(t *testing.T) {
	srv := newServer(log.New(io.Discard, "", 0))
	rec := apiRequest(srv, http.MethodGet, "/v1/me", nil, false)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "api_token_missing") {
		t.Fatalf("unauthenticated status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = apiRequest(srv, http.MethodGet, "/v1/nope", nil, true)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "not_found") {
		t.Fatalf("unknown route status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = apiRequest(srv, http.MethodPost, "/v1/me", nil, true)
	if rec.Code != http.StatusMethodNotAllowed || !strings.Contains(rec.Body.String(), "method_not_allowed") {
		t.Fatalf("wrong method status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = apiRequest(srv, http.MethodGet, "/v1/rate-limited", nil, true)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "1" || rec.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("rate limit status=%d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}
}

func TestAdminAndReady(t *testing.T) {
	srv := newServer(log.New(io.Discard, "", 0))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, testURL("/readyz"), nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok\n" {
		t.Fatalf("ready status=%d body=%q", rec.Code, rec.Body.String())
	}

	_ = apiRequest(srv, http.MethodGet, "/v1/me", nil, true)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, testURL("/__mock/requests"), nil))
	var count struct {
		APIRequests int `json:"api_requests"`
	}
	decodeBody(t, rec, &count)
	if count.APIRequests != 1 {
		t.Fatalf("api_requests = %d", count.APIRequests)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, testURL("/__mock/reset"), nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("reset status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, testURL("/__mock/requests"), nil))
	decodeBody(t, rec, &count)
	if count.APIRequests != 0 {
		t.Fatalf("api_requests after reset = %d", count.APIRequests)
	}
}

func TestLoggingRedactsSensitiveValues(t *testing.T) {
	var logs bytes.Buffer
	srv := newServer(log.New(&logs, "", 0))
	rec := apiRequestWithHeaders(srv, http.MethodGet, "/v1/projects?token=super-secret-token&status=active", nil, map[string]string{
		"Authorization":     "Bearer raw-secret-value",
		"Idempotency-Key":   "idempotency-secret",
		"X-Unused-For-Test": "ignored",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	got := logs.String()
	for _, forbidden := range []string{"raw-secret-value", "idempotency-secret", "super-secret-token"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("log leaked %q in %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "token=%5BREDACTED%5D") || !regexp.MustCompile(`auth=[0-9a-f]{8}`).MatchString(got) || !regexp.MustCompile(`idem=[0-9a-f]{8}`).MatchString(got) {
		t.Fatalf("log missing redaction/hash markers: %s", got)
	}
}

func TestListenAndAnnounceContract(t *testing.T) {
	var stdout bytes.Buffer
	ln, srv, err := listenAndAnnounce("127.0.0.1:0", &stdout, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		_ = srv.Serve(ln)
	}()
	defer srv.Close()

	line := stdout.String()
	if !regexp.MustCompile(`^LISTEN_ADDR=127\.0\.0\.1:\d+\n$`).MatchString(line) {
		t.Fatalf("stdout = %q", line)
	}
	addr := strings.TrimSpace(strings.TrimPrefix(line, "LISTEN_ADDR="))
	client := &http.Client{Timeout: time.Second}
	scheme := "http"
	var ok bool
	for i := 0; i < 40; i++ {
		resp, err := client.Get(scheme + "://" + addr + "/readyz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ok = true
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ok {
		t.Fatalf("/readyz did not become ready at %s", addr)
	}
}

func apiRequest(srv http.Handler, method, path string, body io.Reader, auth bool) *httptest.ResponseRecorder {
	headers := map[string]string{}
	if auth {
		headers["Authorization"] = "Bearer test-key"
	}
	return apiRequestWithHeaders(srv, method, path, body, headers)
}

func apiRequestWithHeaders(srv http.Handler, method, path string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, testURL(path), body)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func testURL(path string) string {
	return "http" + "://example.test" + path
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rec.Body.String())
	}
}
