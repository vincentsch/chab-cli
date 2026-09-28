package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostMultipartStreamsFileFieldsAndIdempotency(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(source, []byte("hello upload"), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/files" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fixture" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Idempotency-Key"); got != "idem-upload" {
			t.Fatalf("Idempotency-Key = %q", got)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data; boundary=") {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error = %v", err)
		}
		if got := r.FormValue("project_id"); got != "42" {
			t.Fatalf("project_id = %q", got)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile error = %v", err)
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("ReadAll error = %v", err)
		}
		if string(data) != "hello upload" || header.Filename != "contract.txt" {
			t.Fatalf("upload = filename %q body %q", header.Filename, string(data))
		}
		fmt.Fprint(w, `{"data":{"file":{"id":"fil_123","state":"scan_pending"},"next_check_at":"2026-09-13T10:00:05Z"},"meta":{"request_id":"req-upload"}}`)
	}))
	defer server.Close()

	client, err := New(Options{BaseURL: server.URL + "/api/v1", APIKey: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.PostMultipart(context.Background(), "files", MultipartFileUpload{
		FilePath:      source,
		FileFieldName: "file",
		Filename:      "contract.txt",
		Fields:        map[string]string{"project_id": "42"},
		MaxBytes:      100,
		Idempotency:   ExplicitIdempotency("idem-upload"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Meta.RequestID != "req-upload" || !strings.Contains(string(result.Data), `"fil_123"`) {
		t.Fatalf("result = %#v data=%s", result.Meta, result.Data)
	}
}

func TestPostMultipartRejectsOversizedFileBeforeHTTP(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(source, []byte("too large"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("oversized upload contacted HTTP server")
	}))
	defer server.Close()
	client, err := New(Options{BaseURL: server.URL + "/api/v1", APIKey: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.PostMultipart(context.Background(), "files", MultipartFileUpload{
		FilePath:    source,
		MaxBytes:    3,
		Idempotency: ExplicitIdempotency("idem-upload"),
	}); err == nil {
		t.Fatal("expected oversized file error")
	}
}
