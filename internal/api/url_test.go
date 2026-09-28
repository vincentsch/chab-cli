package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/api"
)

func TestPathEscapesOpaqueSegments(t *testing.T) {
	got := api.Path("", "projects", "id/with?bad#frag", "snow man", "ä", ".", "..")
	want := "projects/id%2Fwith%3Fbad%23frag/snow%20man/%C3%A4/%2E/%2E%2E"
	if got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestNormalizeURLForMatch(t *testing.T) {
	httpDefault, err := api.NormalizeURLForMatch("http://localhost:80/api/v1/")
	if err != nil {
		t.Fatalf("NormalizeURLForMatch(http default) error = %v", err)
	}
	httpPlain, err := api.NormalizeURLForMatch("http://localhost/api/v1")
	if err != nil {
		t.Fatalf("NormalizeURLForMatch(http plain) error = %v", err)
	}
	if httpDefault != httpPlain {
		t.Fatalf("default port/trailing slash normalization = %q vs %q", httpDefault, httpPlain)
	}

	httpsDefault, err := api.NormalizeURLForMatch("https://example.test:443/api/v1/")
	if err != nil {
		t.Fatalf("NormalizeURLForMatch(https default) error = %v", err)
	}
	if httpsDefault != "https://example.test/api/v1" {
		t.Fatalf("https default normalization = %q", httpsDefault)
	}

	localhost, err := api.NormalizeURLForMatch("http://localhost/api/v1")
	if err != nil {
		t.Fatalf("NormalizeURLForMatch(localhost) error = %v", err)
	}
	loopback, err := api.NormalizeURLForMatch("http://127.0.0.1/api/v1")
	if err != nil {
		t.Fatalf("NormalizeURLForMatch(loopback) error = %v", err)
	}
	if localhost == loopback {
		t.Fatalf("localhost and 127.0.0.1 normalized to the same value %q", localhost)
	}

	portMismatch, err := api.NormalizeURLForMatch("http://localhost:8020/api/v1")
	if err != nil {
		t.Fatalf("NormalizeURLForMatch(port mismatch) error = %v", err)
	}
	if portMismatch == httpPlain {
		t.Fatalf("explicit non-default port matched default: %q", portMismatch)
	}
	pathMismatch, err := api.NormalizeURLForMatch("http://localhost/api/v2")
	if err != nil {
		t.Fatalf("NormalizeURLForMatch(path mismatch) error = %v", err)
	}
	if pathMismatch == httpPlain {
		t.Fatalf("different paths matched: %q", pathMismatch)
	}
}

func TestNormalizeURLForMatchRejectsAmbiguousURLs(t *testing.T) {
	for _, raw := range []string{
		"https://user:pass@example.test/api/v1",
		"https://example.test/api/v1?x=1",
		"https://example.test/api/v1?",
		"https://example.test/api/v1#frag",
		"https://example.test/api/v1#",
		"ftp://example.test/api/v1",
		"https:///api/v1",
		"api/v1",
	} {
		t.Run(strings.ReplaceAll(raw, "/", "_"), func(t *testing.T) {
			if _, err := api.NormalizeURLForMatch(raw); err == nil {
				t.Fatalf("NormalizeURLForMatch(%q) succeeded", raw)
			}
		})
	}
}

func TestRequestURLPreservesBasePathEscapesAndQuery(t *testing.T) {
	var requestURI string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURI = r.RequestURI
		writeJSON(w, http.StatusOK, `{"data":{"ok":true}}`)
	}))
	defer server.Close()

	client, _, _ := newTestClient(t, server.URL+"/v1", nil)
	query := url.Values{
		"q":     {"x/y?z"},
		"space": {"a b"},
	}
	var out map[string]bool
	if _, err := client.Get(context.Background(), api.Path("projects", "a/b?#"), query, &out); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	want := "/v1/projects/a%2Fb%3F%23?q=x%2Fy%3Fz&space=a+b"
	if requestURI != want {
		t.Fatalf("RequestURI = %q, want %q", requestURI, want)
	}
}

func TestRequestURLPreservesEscapedDotSegments(t *testing.T) {
	var requestURI string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURI = r.RequestURI
		writeJSON(w, http.StatusOK, `{"data":{"ok":true}}`)
	}))
	defer server.Close()

	client, _, _ := newTestClient(t, server.URL+"/v1", nil)
	var out map[string]bool
	if _, err := client.Get(context.Background(), api.Path("projects", ".."), nil, &out); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if requestURI != "/v1/projects/%2E%2E" {
		t.Fatalf("RequestURI = %q, want escaped dot segment", requestURI)
	}
}

func TestRequestURLAvoidsDoubleSlashes(t *testing.T) {
	var requestURI string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURI = r.RequestURI
		writeJSON(w, http.StatusOK, `{"data":{"ok":true}}`)
	}))
	defer server.Close()

	client, _, _ := newTestClient(t, server.URL+"/api/v1/", nil)
	var out map[string]bool
	if _, err := client.Get(context.Background(), "/me", nil, &out); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if requestURI != "/api/v1/me" {
		t.Fatalf("RequestURI = %q, want /api/v1/me", requestURI)
	}
}
