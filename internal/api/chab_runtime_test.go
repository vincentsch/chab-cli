package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/api"
)

func TestChabBootstrapCompatibilityAndOAuth(t *testing.T) {
	var sawCompatibilityETag bool
	var sawDeviceForm bool
	var sawTokenForm bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cli/compatibility":
			if r.Header.Get("If-None-Match") == `"compat-1"` {
				sawCompatibilityETag = true
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"compat-1"`)
			writeChabJSON(t, w, map[string]any{
				"data": map[string]any{
					"schema_version":      "1.0.0",
					"api_major":           1,
					"minimum_version":     "1.0.0",
					"recommended_version": "1.2.0",
					"catalog_version":     "cat-1",
					"blocked_ranges":      []any{},
					"notices":             []any{},
					"urls":                map[string]string{"docs": "https://example.test/docs"},
				},
				"meta": map[string]any{"request_id": "req-compat"},
			})
		case "/oauth/device/code":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			want := url.Values{
				"client_id":      []string{"chab-cli"},
				"client_version": []string{"1.0.0"},
				"scope":          []string{"api:projects:read api:credits:read"},
				"device_name":    []string{"workstation"},
			}
			if r.Form.Encode() != want.Encode() {
				t.Fatalf("device form = %s, want %s", r.Form.Encode(), want.Encode())
			}
			sawDeviceForm = true
			writeChabJSON(t, w, map[string]any{
				"device_code":               "dev-secret",
				"user_code":                 "ABCD-EFGH",
				"verification_uri":          chabServerURL(r) + "/verify",
				"verification_uri_complete": chabServerURL(r) + "/verify?user_code=ABCD-EFGH",
				"expires_in":                600,
				"interval":                  2,
			})
		case "/oauth/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			want := url.Values{
				"grant_type":  []string{api.DeviceCodeGrantType},
				"client_id":   []string{"chab-cli"},
				"device_code": []string{"dev-secret"},
			}
			if r.Form.Encode() != want.Encode() {
				t.Fatalf("token form = %s, want %s", r.Form.Encode(), want.Encode())
			}
			sawTokenForm = true
			writeChabJSON(t, w, map[string]any{"access_token": "api-secret", "token_type": "Bearer", "scope": "api:projects:read"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := api.NewBootstrap(api.BootstrapOptions{AppBaseURL: server.URL, APIBaseURL: server.URL + "/v1", UserAgentVersion: "dev"})
	if err != nil {
		t.Fatalf("NewBootstrap: %v", err)
	}
	if got := api.EffectiveClientVersion("dev"); got != api.DevelopmentClientVersion {
		t.Fatalf("EffectiveClientVersion(dev) = %q", got)
	}
	if _, meta, err := client.Compatibility(context.Background(), "1.0.0"); err != nil || meta.RequestID != "req-compat" {
		t.Fatalf("Compatibility() = meta %#v err %v", meta, err)
	}
	if _, _, err := client.Compatibility(context.Background(), "1.0.0"); err != nil {
		t.Fatalf("cached Compatibility(): %v", err)
	}
	if !sawCompatibilityETag {
		t.Fatalf("second compatibility request did not revalidate with ETag")
	}
	device, _, err := client.CreateDevice(context.Background(), "chab-cli", "1.0.0", []string{"api:projects:read", "api:credits:read"}, "workstation")
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	token, _, err := client.PollToken(context.Background(), device.DeviceCode)
	if err != nil {
		t.Fatalf("PollToken: %v", err)
	}
	if !sawDeviceForm || !sawTokenForm || token.AccessToken != "api-secret" {
		t.Fatalf("missing OAuth calls: device=%t token=%t token=%#v", sawDeviceForm, sawTokenForm, token)
	}
}

func TestChabAcceptedOperationEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/operations" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		writeJSON(w, http.StatusAccepted, `{"operation":{"id":"op_123","status":"queued"},"meta":{"request_id":"req-op"}}`)
	}))
	defer server.Close()

	client, _, _ := newTestClient(t, server.URL+"/v1", nil)
	var typed struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if meta, err := client.Post(context.Background(), "operations", nil, map[string]string{"name": "demo"}, api.ExplicitIdempotency("idem-operation"), &typed); err != nil {
		t.Fatalf("Post() error = %v", err)
	} else if typed.ID != "op_123" || typed.Status != "queued" || meta.RequestID != "req-op" {
		t.Fatalf("typed operation = %#v meta=%#v", typed, meta)
	}

	raw, err := client.DoRaw(context.Background(), http.MethodPost, "operations", nil, map[string]string{"name": "demo"}, api.ExplicitIdempotency("idem-operation-raw"))
	if err != nil {
		t.Fatalf("DoRaw() error = %v", err)
	}
	if string(raw.Data) != `{"id":"op_123","status":"queued"}` || !strings.Contains(string(raw.Envelope), `"operation"`) {
		t.Fatalf("raw operation = data:%s envelope:%s", raw.Data, raw.Envelope)
	}
}

func TestChabRejectsMalformedOperationEnvelopes(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "null operation",
			body: `{"operation":null,"meta":{"request_id":"req-op"}}`,
			want: "operation is null",
		},
		{
			name: "data and operation",
			body: `{"data":{"id":"data"},"operation":{"id":"op_123"},"meta":{"request_id":"req-op"}}`,
			want: "both data and operation",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, http.StatusAccepted, tc.body)
			}))
			defer server.Close()

			client, _, _ := newTestClient(t, server.URL+"/v1", nil)
			_, err := client.DoRaw(context.Background(), http.MethodPost, "operations", nil, map[string]string{"name": "demo"}, api.ExplicitIdempotency("idem-operation-raw"))
			var protoErr *api.ProtocolError
			if !errors.As(err, &protoErr) || !strings.Contains(protoErr.Detail, tc.want) {
				t.Fatalf("DoRaw() error = %T %v, want protocol detail containing %q", err, err, tc.want)
			}
		})
	}
}

func TestChabErrorDetailsRedactNumericSecretsAndPreserveNull(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnprocessableEntity, `{"error":{"code":"validation_failed","message":"Invalid.","retryable":false,"details":{"pin":123456,"big":9007199254740993,"flag":true,"explicit":null}},"request_id":"req-details"}`)
	}))
	defer server.Close()

	client, _, _ := newTestClient(t, server.URL+"/v1", func(opts *api.Options) {
		opts.SecretValues = append(opts.SecretValues, "123456")
	})
	var out map[string]any
	_, err := client.Get(context.Background(), "me", nil, &out)
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("Get() error = %T %v, want api.Error", err, err)
	}
	details, ok := apiErr.DetailsValue.(map[string]any)
	if !ok {
		t.Fatalf("DetailsValue = %#v", apiErr.DetailsValue)
	}
	if details["pin"] != "[REDACTED]" {
		t.Fatalf("pin detail = %#v", details["pin"])
	}
	if number, ok := details["big"].(json.Number); !ok || number.String() != "9007199254740993" {
		t.Fatalf("big detail = %#v (%T)", details["big"], details["big"])
	}
	if _, ok := details["explicit"]; !ok || details["explicit"] != nil {
		t.Fatalf("explicit null detail = %#v", details)
	}
	if !apiErr.DetailsPresent || !strings.Contains(string(apiErr.RawDetails), `"big":9007199254740993`) {
		t.Fatalf("details presence/raw = present:%t raw:%s", apiErr.DetailsPresent, apiErr.RawDetails)
	}
}

func TestChabResolveCanonicalAPILink(t *testing.T) {
	client, _, _ := newTestClient(t, "https://api.example.test/custom/v1", nil)

	got, err := client.ResolveAPILink("/v1/operations/op_1/result?include=events")
	if err != nil {
		t.Fatalf("ResolveAPILink() error = %v", err)
	}
	want := "https://api.example.test/custom/v1/operations/op_1/result?include=events"
	if got != want {
		t.Fatalf("ResolveAPILink() = %q, want %q", got, want)
	}

	for _, raw := range []string{
		"https://api.example.test/v1/me",
		"//api.example.test/v1/me",
		"/api/v1/me",
		"/v1/../me",
		"/v1/%2E/me",
		"/v1/id%2Fwithslash",
		"/v1/me#frag",
		" /v1/me",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := client.ResolveAPILink(raw); err == nil {
				t.Fatalf("ResolveAPILink(%q) succeeded", raw)
			}
		})
	}
}

func writeChabJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
}

func chabServerURL(r *http.Request) string {
	return "http://" + r.Host
}
