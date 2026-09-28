package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGitHubFetcherSuccess(t *testing.T) {
	var gotPath, gotAccept, gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		gotAccept = r.Header.Get("Accept")
		gotUA = r.Header.Get("User-Agent")
		if err := json.NewEncoder(w).Encode([]Release{
			{Tag: "v0.6.0"},
			{Tag: "v0.7.0-rc1", Prerelease: true},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	releases, err := NewGitHubFetcher(server.URL, time.Second)(context.Background())
	if err != nil {
		t.Fatalf("fetch returned error: %v", err)
	}
	if gotPath != "/repos/"+Repository+"/releases?per_page=30" {
		t.Fatalf("request URI = %q", gotPath)
	}
	if gotAccept != "application/vnd.github+json" {
		t.Fatalf("Accept = %q", gotAccept)
	}
	if gotUA != "chab-version-check" {
		t.Fatalf("User-Agent = %q", gotUA)
	}
	if len(releases) != 2 || releases[0].Tag != "v0.6.0" || !releases[1].Prerelease {
		t.Fatalf("releases = %#v", releases)
	}
}

func TestGitHubFetcherMalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "bad body", body: `{`},
		{name: "trailing garbage", body: `[{"tag_name":"v0.6.0"}] garbage`},
		{name: "extra value", body: `[{"tag_name":"v0.6.0"}] []`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			_, err := NewGitHubFetcher(server.URL, time.Second)(context.Background())
			if err == nil || !strings.Contains(err.Error(), "decode GitHub releases") {
				t.Fatalf("err = %v, want decode error", err)
			}
		})
	}
}

func TestGitHubFetcherNon2xx(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, http.StatusText(status), status)
			}))
			defer server.Close()

			_, err := NewGitHubFetcher(server.URL, time.Second)(context.Background())
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("status %d", status)) {
				t.Fatalf("err = %v, want status error", err)
			}
		})
	}
}

func TestGitHubFetcherTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	_, err := NewGitHubFetcher(server.URL, time.Millisecond)(context.Background())
	if err == nil || !strings.Contains(err.Error(), "contact GitHub releases") {
		t.Fatalf("err = %v, want timeout transport error", err)
	}
}
