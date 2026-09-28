package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/api"
)

func TestDebugOutputRedactsSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, `{"error":{"code":"invalid_api_token","message":"`+authorizationFixture("server-secret")+`"},"request_id":"req-envelope"}`)
	}))
	defer server.Close()

	client, debug, _ := newTestClient(t, server.URL+"/api/v1", nil)
	var out map[string]bool
	_, _ = client.Get(context.Background(), "whoami?echo=ak_test|query-secret", nil, &out)

	debugOutput := debug.String()
	for _, leaked := range []string{"super-secret", "query-secret", "server-secret", "Authorization: Bearer " + fakeAPIKey} {
		if strings.Contains(debugOutput, leaked) {
			t.Fatalf("debug leaked %q in %s", leaked, debugOutput)
		}
	}
	if !strings.Contains(debugOutput, "[REDACTED]") {
		t.Fatalf("debug did not include redaction marker: %s", debugOutput)
	}
}

func TestErrorStringsRedactSecrets(t *testing.T) {
	errs := []error{
		&api.Error{Code: "invalid_api_token", Message: "bad ak_test|api-secret", RequestID: "req", Status: 401},
		&api.ProtocolError{Detail: "bad ak_test|protocol-secret", Status: 500, RequestID: "req"},
		&api.TransportError{Err: errors.New(authorizationFixture("transport-secret")), Attempts: 1},
		&api.UsageError{Field: "api_key", Detail: "bad ak_test|usage-secret"},
	}
	for _, err := range errs {
		got := err.Error()
		for _, leaked := range []string{"api-secret", "protocol-secret", "transport-secret", "usage-secret"} {
			if strings.Contains(got, leaked) {
				t.Fatalf("%T leaked %q in %q", err, leaked, got)
			}
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Fatalf("%T did not redact: %q", err, got)
		}
	}
}

func authorizationFixture(secret string) string {
	return "Authorization: " + "Bearer ak_test|" + secret
}
