package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestCompatibilityRejectsClientBelowMinimum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeBootstrapJSON(t, w, map[string]any{
			"data": map[string]any{
				"schema_version":      "1.0.0",
				"api_major":           1,
				"minimum_version":     "1.5.0",
				"recommended_version": "1.6.0",
				"catalog_version":     "cat-1",
				"blocked_ranges":      []any{},
				"notices":             []any{},
			},
			"meta": map[string]any{"request_id": "req-minimum"},
		})
	}))
	defer server.Close()

	client, err := NewBootstrap(BootstrapOptions{
		AppBaseURL:       server.URL + "/app",
		APIBaseURL:       server.URL + "/custom/v1",
		UserAgentVersion: "1.0.0",
		Now:              func() time.Time { return time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("NewBootstrap() error = %v", err)
	}
	_, _, err = client.Compatibility(context.Background(), "1.0.0")
	usage, ok := err.(*UsageError)
	if !ok || usage.Field != "client_version" {
		t.Fatalf("Compatibility() error = %T %v, want client_version usage", err, err)
	}
}

func TestCompatibilityKeepsGuestAndFutureHostedMetadata(t *testing.T) {
	var data CompatibilityData
	if err := json.Unmarshal([]byte(`{"local_mcp":{"transport":"stdio","api_transport":"rest","guest_trial_credentials":{"accepted":true,"enabled":true,"tools":[],"future_guest_limit":3}},"hosted_mcp":{"requires_verified_account":false,"guest_credentials_accepted":true,"operation_transport":true,"advertised_operation_keys":["search.web"],"future_host_field":"kept"}}`), &data); err != nil {
		t.Fatal(err)
	}
	if guest, err := data.GuestLocalMCP(); err != nil || !guest.Accepted {
		t.Fatalf("GuestLocalMCP = %#v, %v", guest, err)
	}
	if hosted, err := data.HostedMCPInfo(); err != nil || !hosted.GuestCredentialsAccepted || !hosted.OperationTransport || hosted.RequiresVerifiedAccount {
		t.Fatalf("HostedMCPInfo = %#v, %v", hosted, err)
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"future_guest_limit":3`)) || !bytes.Contains(encoded, []byte(`"future_host_field":"kept"`)) {
		t.Fatalf("compatibility lost future fields: %s", encoded)
	}
}

func TestCompatibilityDiskCacheRevalidatesAcrossClients(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	cachePath := filepath.Join(t.TempDir(), "compatibility-cache.json")
	var sawRevalidation bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tenant/v1/cli/compatibility" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("If-None-Match") == `"etag-1"` {
			sawRevalidation = true
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"etag-1"`)
		writeBootstrapJSON(t, w, map[string]any{
			"data": compatibilityPayload(),
			"meta": map[string]any{"request_id": "req-cache"},
		})
	}))
	defer server.Close()

	first, err := NewBootstrap(BootstrapOptions{
		AppBaseURL:             server.URL + "/app",
		APIBaseURL:             server.URL + "/tenant/v1",
		UserAgentVersion:       "1.2.3",
		CompatibilityCachePath: cachePath,
		Now:                    func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewBootstrap(first) error = %v", err)
	}
	if _, _, err := first.Compatibility(context.Background(), "1.2.3"); err != nil {
		t.Fatalf("first Compatibility() error = %v", err)
	}

	key := compatibilityCacheKey(server.URL+"/tenant/v1", "1.2.3")
	compatibilityCache.Delete(key)
	second, err := NewBootstrap(BootstrapOptions{
		AppBaseURL:             server.URL + "/app",
		APIBaseURL:             server.URL + "/tenant/v1",
		UserAgentVersion:       "1.2.3",
		CompatibilityCachePath: cachePath,
		Now:                    func() time.Time { return now.Add(time.Minute) },
	})
	if err != nil {
		t.Fatalf("NewBootstrap(second) error = %v", err)
	}
	data, meta, err := second.Compatibility(context.Background(), "1.2.3")
	if err != nil {
		t.Fatalf("second Compatibility() error = %v", err)
	}
	if !sawRevalidation || data.CatalogVersion != "cat-1" || meta.HTTPStatus != http.StatusNotModified {
		t.Fatalf("revalidation=%t data=%#v meta=%#v", sawRevalidation, data, meta)
	}
}

func compatibilityPayload() map[string]any {
	return map[string]any{
		"schema_version":      "1.0.0",
		"api_major":           1,
		"minimum_version":     "1.0.0",
		"recommended_version": "1.2.0",
		"catalog_version":     "cat-1",
		"blocked_ranges":      []any{},
		"notices":             []any{},
	}
}

func writeBootstrapJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
}
