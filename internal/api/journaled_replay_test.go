package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestJournaledReplayMakesOnePhysicalRequest(t *testing.T) {
	for _, tc := range []struct {
		name      string
		multipart bool
		headers   bool
		failure   string
	}{
		{"json_429", false, false, "429"},
		{"json_headers_429", false, true, "429"},
		{"json_transport", false, false, "transport"},
		{"multipart_429", true, false, "429"},
		{"multipart_transport", true, false, "transport"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/v1/me" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"data":{"id":"test"}}`)
					return
				}
				calls.Add(1)
				if r.Header.Get("Idempotency-Key") != "idem-replay-once" {
					t.Errorf("idempotency key = %q", r.Header.Get("Idempotency-Key"))
				}
				if tc.failure == "transport" {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Errorf("hijack: %v", err)
						return
					}
					_ = conn.Close()
					return
				}
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, `{"error":{"code":"rate_limited","message":"wait"}}`)
			}))
			defer server.Close()
			client, err := New(Options{BaseURL: server.URL + "/v1", APIKey: "test_key"})
			if err != nil {
				t.Fatal(err)
			}
			if tc.failure == "transport" {
				if _, err := client.DoRaw(context.Background(), http.MethodGet, "me", nil, nil, IdempotencyNone); err != nil {
					t.Fatalf("prime ordinary keep-alive connection: %v", err)
				}
			}
			if tc.multipart {
				path := filepath.Join(t.TempDir(), "upload.txt")
				if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
					t.Fatal(err)
				}
				_, err = client.PostMultipart(context.Background(), "files", MultipartFileUpload{FilePath: path, FileFieldName: "file", MaxBytes: 100, Idempotency: JournaledIdempotency("idem-replay-once", true)})
			} else if tc.headers {
				_, err = client.DoRawWithHeaders(context.Background(), http.MethodPost, "search/web", nil, ExactJSONBody(`{"query":"x"}`), JournaledIdempotency("idem-replay-once", true), http.Header{"X-Chab-Approval-Proof": []string{"test-proof"}})
			} else {
				_, err = client.DoRaw(context.Background(), http.MethodPost, "search/web", nil, ExactJSONBody(`{"query":"x"}`), JournaledIdempotency("idem-replay-once", true))
			}
			if err == nil || calls.Load() != 1 {
				t.Fatalf("replay err=%v, physical requests=%d, want one", err, calls.Load())
			}
		})
	}
}

func TestJournaledReplayForcesHTTP1EvenWhenServerOffersHTTP2(t *testing.T) {
	var proto atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proto.Store(int32(r.ProtoMajor))
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":"rate_limited","message":"wait"}}`)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client, err := New(Options{BaseURL: server.URL + "/v1", APIKey: "test_key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.DoRaw(context.Background(), http.MethodPost, "search/web", nil, ExactJSONBody(`{"query":"x"}`), JournaledIdempotency("idem-h1-replay", true))
	if err == nil || proto.Load() != 1 {
		t.Fatalf("replay err=%v, negotiated HTTP/%d; require HTTP/1 to avoid HTTP/2 stream replay", err, proto.Load())
	}
}

func TestConcurrentJournaledReplaySharesDebugWriterLock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":"rate_limited","message":"wait"}}`)
	}))
	defer server.Close()
	var debug bytes.Buffer
	client, err := New(Options{BaseURL: server.URL + "/v1", APIKey: "test_key", DebugWriter: &debug})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, _ = client.DoRaw(context.Background(), http.MethodPost, "search/web", nil, ExactJSONBody(`{"query":"x"}`), JournaledIdempotency("idem-debug-replay", true))
		}()
	}
	workers.Wait()
	if debug.Len() == 0 {
		t.Fatal("expected redacted debug lines")
	}
}

func TestJournaledReplayFailsClosedForUncloneableTransport(t *testing.T) {
	client, err := New(Options{BaseURL: "https://api.example.test/v1", APIKey: "test_key", Transport: &recordingRoundTripper{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.DoRaw(context.Background(), http.MethodPost, "search/web", nil, ExactJSONBody(`{"query":"x"}`), JournaledIdempotency("idem-replay-once", true))
	if err == nil {
		t.Fatal("uncloneable transport must fail closed before HTTP")
	}
}
