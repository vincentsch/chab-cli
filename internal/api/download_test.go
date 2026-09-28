package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadStreamsBoundedBytesAndNeverFollowsRedirects(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("missing auth")
		}
		switch r.URL.Path {
		case "/api/v1/source":
			w.Header().Set("Content-Type", "message/rfc822")
			fmt.Fprint(w, "private bytes")
		case "/api/v1/large":
			fmt.Fprint(w, "larger than cap")
		case "/api/v1/redirect":
			w.Header().Set("Location", "/api/v1/source")
			w.WriteHeader(302)
		}
	}))
	defer server.Close()
	client, err := New(Options{BaseURL: server.URL + "/api/v1", APIKey: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	n, meta, err := client.Download(context.Background(), "source", nil, &out, 100)
	if err != nil || n != 13 || out.String() != "private bytes" || meta.Attempts != 1 {
		t.Fatalf("%d %#v %v %q", n, meta, err, out.String())
	}
	out.Reset()
	if _, _, err := client.Download(context.Background(), "large", nil, &out, 3); err == nil || out.Len() > 3 {
		t.Fatalf("limit: %v %d", err, out.Len())
	}
	out.Reset()
	if _, _, err := client.Download(context.Background(), "redirect", nil, &out, 100); err == nil || out.Len() != 0 {
		t.Fatalf("redirect: %v %d", err, out.Len())
	}
	if calls != 3 {
		t.Fatalf("retried or followed redirect: %d", calls)
	}
}

func TestDownloadOptInRedirectStripsCredentialsAndRejectsUnsafeTargets(t *testing.T) {
	var redirectedAuth, redirectedCookie, publicURL string
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedAuth = r.Header.Get("Authorization")
		redirectedCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/octet-stream")
		fmt.Fprint(w, "redirected bytes")
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/redirect":
			if r.Header.Get("Authorization") != "Bearer fixture" {
				t.Error("missing initial auth")
			}
			w.Header().Set("Location", publicURL)
			w.WriteHeader(http.StatusFound)
		case "/api/v1/private":
			w.Header().Set("Location", "https://127.0.0.1/private")
			w.WriteHeader(http.StatusFound)
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer source.Close()

	publicURL = strings.Replace(target.URL, "https://127.0.0.1", "https://downloads.example.test", 1)
	transport := target.Client().Transport
	client, err := New(Options{BaseURL: source.URL + "/api/v1", APIKey: "fixture", Transport: rewriteHostTransport{base: transport, targetHost: strings.TrimPrefix(publicURL, "https://"), replacementHost: strings.TrimPrefix(target.URL, "https://")}})
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	n, _, err := client.DownloadWithOptions(context.Background(), "redirect", &out, DownloadOptions{MaxBytes: 100, FollowHTTPSRedirect: true})
	if err != nil || n != 16 || out.String() != "redirected bytes" {
		t.Fatalf("redirect download = n=%d err=%v body=%q", n, err, out.String())
	}
	if redirectedAuth != "" || redirectedCookie != "" {
		t.Fatalf("redirect request carried credentials: auth=%q cookie=%q", redirectedAuth, redirectedCookie)
	}

	out.Reset()
	if _, _, err := client.DownloadWithOptions(context.Background(), "private", &out, DownloadOptions{MaxBytes: 100, FollowHTTPSRedirect: true}); err == nil || out.Len() != 0 {
		t.Fatalf("unsafe redirect = err=%v body=%q", err, out.String())
	}
}

func TestDownloadRedirectTransportErrorRedactsSignedTarget(t *testing.T) {
	signedTarget := "https://downloads.example.test/object?X-Amz-Credential=credential-secret&X-Amz-Signature=signature-secret"
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/redirect" {
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
		w.Header().Set("Location", signedTarget)
		w.WriteHeader(http.StatusFound)
	}))
	defer source.Close()

	client, err := New(Options{
		BaseURL:   source.URL + "/api/v1",
		APIKey:    "fixture",
		Transport: redirectFailureTransport{base: http.DefaultTransport, failHost: "downloads.example.test"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	_, _, err = client.DownloadWithOptions(context.Background(), "redirect", &out, DownloadOptions{MaxBytes: 100, FollowHTTPSRedirect: true})
	if err == nil {
		t.Fatal("redirect download succeeded, want transport error")
	}
	text := err.Error()
	for _, forbidden := range []string{signedTarget, "downloads.example.test", "X-Amz-Credential", "X-Amz-Signature", "credential-secret", "signature-secret"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("redirect transport error leaked %q in %q", forbidden, text)
		}
	}
}

type rewriteHostTransport struct {
	base            http.RoundTripper
	targetHost      string
	replacementHost string
}

func (t rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == t.targetHost {
		copy := req.Clone(req.Context())
		copy.URL.Host = t.replacementHost
		req = copy
	}
	return t.base.RoundTrip(req)
}

type redirectFailureTransport struct {
	base     http.RoundTripper
	failHost string
}

func (t redirectFailureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == t.failHost {
		return nil, fmt.Errorf("redirect GET %s failed", req.URL.String())
	}
	return t.base.RoundTrip(req)
}
