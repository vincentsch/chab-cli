package testutil_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/redact"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestWriteConfigProfileNeverDefaultsTestOriginToProduction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	testutil.WriteConfigProfile(t, path, "local", config.Profile{Locale: "en"}, true)
	file, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	view, _, err := config.ResolveProfileView(file, "local")
	if err != nil || view.BaseURL != "http://127.0.0.1:9" || view.APIBaseURL != "http://127.0.0.1:9/v1" {
		t.Fatalf("test profile escaped loopback: view=%#v err=%v", view, err)
	}
}

func TestFakeKeyMatchesRedactionAndLintPatterns(t *testing.T) {
	key := testutil.FakeKey("abc")
	if key != "ak_abc|ssssssssssssssss" {
		t.Fatalf("FakeKey() = %q", key)
	}
	if !testutil.SecretPairPattern.MatchString(key) {
		t.Fatalf("SecretPairPattern did not match %q", key)
	}
	if got := redact.String("token " + key); got != "token ak_abc|[REDACTED]" {
		t.Fatalf("redact.String() = %q", got)
	}

	for _, id := range []string{"ab", "bad space", "bad/slash"} {
		t.Run(id, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("FakeKey(%q) did not panic", id)
				}
			}()
			_ = testutil.FakeKey(id)
		})
	}
}

func TestHermeticEnvIgnoresProcessEnvironment(t *testing.T) {
	t.Setenv("CHAB_API_KEY", "process")
	lookup := testutil.HermeticEnv(map[string]string{"CHAB_PROFILE": "staging"})
	if _, ok := lookup("CHAB_API_KEY"); ok {
		t.Fatalf("HermeticEnv consulted process environment")
	}
	if got, ok := lookup("CHAB_PROFILE"); !ok || got != "staging" {
		t.Fatalf("CHAB_PROFILE lookup = %q, %v", got, ok)
	}
	if _, ok := testutil.HermeticEnv(nil)("NO_COLOR"); ok {
		t.Fatalf("nil HermeticEnv returned a value")
	}
}

func TestClearViltEnvRemovesHostValues(t *testing.T) {
	names := []string{
		"CHAB_API_KEY",
		"CHAB_PROFILE",
		"CHAB_CONFIG",
		"CHAB_AUTH_FILE",
		"CHAB_BASE_URL",
		"CHAB_API_BASE_URL",
		"CHAB_LOCALE",
		"CHAB_PAGER",
	}
	for _, name := range names {
		t.Setenv(name, "host")
	}

	testutil.ClearViltEnv()

	for _, name := range names {
		if value, ok := os.LookupEnv(name); ok {
			t.Fatalf("%s still set to %q", name, value)
		}
	}
}

func TestEnvelopeBuildersMarshalDocumentedShapes(t *testing.T) {
	success := testutil.Envelope(map[string]string{"id": "g-1"})
	if got := mustJSON(t, success); got != `{"data":{"id":"g-1"}}` {
		t.Fatalf("Envelope JSON = %s", got)
	}
	errEnv := testutil.ErrorEnvelope("validation_failed", "No.", "req-x", map[string][]string{"name": {"required"}})
	obj := testutil.ParseJSONObject(t, mustJSON(t, errEnv))
	errorObj := obj["error"].(map[string]any)
	if errorObj["code"] != "validation_failed" || errorObj["retryable"] != false || obj["request_id"] != "req-x" {
		t.Fatalf("error envelope = %#v", obj)
	}
}

func TestPageCallsPreserveRawAndPresence(t *testing.T) {
	requests := []testutil.CapturedRequest{
		{Query: url.Values{}},
		{Query: url.Values{"page": {""}, "per_page": {"10"}}},
		{Query: url.Values{"page": {"bad"}, "per_page": {""}}},
	}
	got := testutil.PageCalls(requests)
	want := []testutil.PageCall{
		{Page: 1},
		{Page: 1, PerPage: 10, PageRaw: "", PerPageRaw: "10", PageSet: true, PerPageSet: true},
		{Page: 1, PerPage: 0, PageRaw: "bad", PerPageRaw: "", PageSet: true, PerPageSet: true},
	}
	if len(got) != len(want) {
		t.Fatalf("PageCalls len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("PageCalls[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestAssertIdempotencyKeyMatchesUsesRawHeader(t *testing.T) {
	srv := testutil.NewAPIServer(t, testutil.JSONResponse(t, http.StatusOK, testutil.Envelope(map[string]string{"ok": "true"})))
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/gadgets", nil)
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	req.Header.Set(testutil.HeaderIdempotencyKey, "chab-test-idem")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST error = %v", err)
	}
	_ = resp.Body.Close()

	srv.AssertIdempotencyKeyMatches(t, 0, regexp.MustCompile(`^chab-test-idem$`))
	request := srv.Request(t, 0)
	if got := request.Header.Get(testutil.HeaderIdempotencyKey); got != "[REDACTED]" {
		t.Fatalf("safe idempotency header = %q, want redacted", got)
	}
}

func TestSequenceHandlerOrder(t *testing.T) {
	var order []string
	srv := testutil.NewAPIServer(t, testutil.SequenceHandler(t,
		func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "first")
			testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("internal_error", "temporary", "req-1", nil))
		},
		func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "second")
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]string{"ok": "true"}))
		},
	))
	for range 2 {
		resp, err := http.Get(srv.URL)
		if err != nil {
			t.Fatalf("GET error = %v", err)
		}
		_ = resp.Body.Close()
	}
	if got, want := order, []string{"first", "second"}; got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("sequence order = %#v", got)
	}
}

func TestAPIServerConcurrentRecorder(t *testing.T) {
	srv := testutil.NewAPIServer(t, testutil.JSONResponse(t, http.StatusOK, testutil.Envelope(map[string]string{"ok": "true"})))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(srv.URL + "/api/v1/gadgets")
			if err != nil {
				t.Errorf("GET error = %v", err)
				return
			}
			_ = resp.Body.Close()
		}()
	}
	wg.Wait()
	if got := srv.Count(); got != 20 {
		t.Fatalf("request count = %d, want 20", got)
	}
}

func TestAPIServerExactPathAndQueryAssertions(t *testing.T) {
	secret := "ak_request|secret/value?opaque"
	srv := testutil.NewAPIServer(t, testutil.JSONResponse(t, http.StatusOK, testutil.Envelope(map[string]string{"ok": "true"})))
	requestURL := srv.URL + "/api/v1/projects/" + url.PathEscape(secret) + "?search=" + url.QueryEscape(secret) + "&sort=-name"
	resp, err := http.Get(requestURL)
	if err != nil {
		t.Fatalf("GET error = %v", err)
	}
	_ = resp.Body.Close()

	srv.AssertPathSegments(t, 0, []string{"api", "v1", "projects", secret})
	srv.AssertQueryValues(t, 0, "search", []string{secret})
	srv.AssertQueryValues(t, 0, "sort", []string{"-name"})
	srv.AssertQueryValues(t, 0, "missing", nil)
}

func TestNewPagedListServerEmptyDataIsArray(t *testing.T) {
	srv := testutil.NewPagedListServer(t, testutil.PagedListConfig{Path: "/api/v1/projects"})
	resp, err := http.Get(srv.URL + "/api/v1/projects")
	if err != nil {
		t.Fatalf("GET error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("ReadAll error = %v", err)
	}
	if !strings.Contains(string(body), `"data":[]`) || strings.Contains(string(body), `"data":null`) {
		t.Fatalf("empty page shape = %s", body)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	return string(data)
}
