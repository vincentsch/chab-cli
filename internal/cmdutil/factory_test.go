package cmdutil_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
)

var factoryNow = time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

type factorySleeper struct {
	waits []time.Duration
}

func (s *factorySleeper) Sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.waits = append(s.waits, d)
	return nil
}

func TestFactoryAPIClientWiresRuntimeVersionDebugClockAndSleeper(t *testing.T) {
	var stderr bytes.Buffer
	sleeper := &factorySleeper{}
	factory := &cmdutil.Factory{
		Sleeper: sleeper,
		Now:     func() time.Time { return factoryNow },
		Version: "9.8.7",
	}

	attempts := 0
	var gotUserAgent, gotAuth, gotLocale string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		gotUserAgent = r.Header.Get("User-Agent")
		gotAuth = r.Header.Get("Authorization")
		gotLocale = r.Header.Get("Accept-Language")
		if attempts == 1 {
			w.Header().Set("Retry-After", factoryNow.Add(2*time.Second).Format(http.TimeFormat))
			writeFactoryJSON(w, http.StatusTooManyRequests, `{"error":{"code":"rate_limited","message":"slow down","retryable":true}}`)
			return
		}
		writeFactoryJSON(w, http.StatusOK, `{"data":{"ok":true}}`)
	}))
	defer server.Close()

	cmd := factoryCommand(true, &stderr)
	client, err := factory.APIClient(factoryRuntime(server.URL), factoryCredential(), cmd)
	if err != nil {
		t.Fatalf("APIClient() error = %v", err)
	}

	var out map[string]bool
	meta, err := client.Get(context.Background(), "whoami", nil, &out)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if attempts != 2 || meta.Attempts != 2 || len(sleeper.waits) != 1 || sleeper.waits[0] != 2*time.Second {
		t.Fatalf("attempts=%d meta=%#v waits=%v", attempts, meta, sleeper.waits)
	}
	if gotUserAgent != "chab/9.8.7" || gotAuth != "Bearer "+factoryCredential().APIKey || gotLocale != "de" {
		t.Fatalf("headers user-agent=%q auth=%q locale=%q", gotUserAgent, gotAuth, gotLocale)
	}
	if !strings.Contains(stderr.String(), "debug: api: request GET") || strings.Contains(stderr.String(), factoryCredential().APIKey) {
		t.Fatalf("debug output = %q", stderr.String())
	}
}

func TestFactoryAPIClientNoRetryUsesSingleAttempt(t *testing.T) {
	sleeper := &factorySleeper{}
	factory := &cmdutil.Factory{
		Sleeper: sleeper,
		Now:     func() time.Time { return factoryNow },
		Version: "9.8.7",
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		writeFactoryJSON(w, http.StatusInternalServerError, `{"error":{"code":"internal_error","message":"temporary","retryable":true}}`)
	}))
	defer server.Close()

	var stderr bytes.Buffer
	client, err := factory.APIClientNoRetry(factoryRuntime(server.URL), factoryCredential(), factoryCommand(true, &stderr))
	if err != nil {
		t.Fatalf("APIClientNoRetry() error = %v", err)
	}

	var out map[string]bool
	_, err = client.Get(context.Background(), "whoami", nil, &out)
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("Get() error = %T %v, want *api.Error", err, err)
	}
	if attempts != 1 || apiErr.Meta.Attempts != 1 || len(sleeper.waits) != 0 {
		t.Fatalf("attempts=%d meta=%#v waits=%v", attempts, apiErr.Meta, sleeper.waits)
	}
	if !strings.Contains(stderr.String(), "retry refused reason=attempt_cap") {
		t.Fatalf("debug output missing attempt cap refusal: %q", stderr.String())
	}
}

func TestFactoryAPIClientConfiguredAppliesOptionHook(t *testing.T) {
	const extraSecret = "request-body-secret"
	const opaqueAPIKey = "opaque-env-api-key"
	factory := &cmdutil.Factory{
		Sleeper: &factorySleeper{},
		Now:     func() time.Time { return factoryNow },
		Version: "9.8.7",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeFactoryJSON(w, http.StatusUnprocessableEntity, `{"error":{"code":"validation_failed","message":"bad request-body-secret and opaque-env-api-key","retryable":false,"details":{"token":["request-body-secret"],"api_key":["opaque-env-api-key"]}}}`)
	}))
	defer server.Close()

	var stderr bytes.Buffer
	client, err := factory.APIClientConfigured(factoryRuntime(server.URL), factoryCredentialWithKey(opaqueAPIKey), factoryCommand(false, &stderr), func(opts *api.Options) {
		opts.SecretValues = []string{extraSecret}
	})
	if err != nil {
		t.Fatalf("APIClientConfigured() error = %v", err)
	}

	var out map[string]bool
	_, err = client.Post(context.Background(), "projects", nil, map[string]string{"token": extraSecret}, api.IdempotencyNone, &out)
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("Post() error = %T %v, want *api.Error", err, err)
	}
	detailsText := strings.Join(apiErr.Details["token"], " ")
	apiKeyDetailsText := strings.Join(apiErr.Details["api_key"], " ")
	errorText := apiErr.Error()
	for _, leaked := range []string{extraSecret, opaqueAPIKey} {
		if strings.Contains(apiErr.Message, leaked) || strings.Contains(detailsText, leaked) ||
			strings.Contains(apiKeyDetailsText, leaked) || strings.Contains(errorText, leaked) {
			t.Fatalf("configured client leaked %q: message=%q details=%q api_key_details=%q error=%q", leaked, apiErr.Message, detailsText, apiKeyDetailsText, errorText)
		}
	}
	if !strings.Contains(apiErr.Message, "[REDACTED]") ||
		!strings.Contains(detailsText, "[REDACTED]") ||
		!strings.Contains(apiKeyDetailsText, "[REDACTED]") {
		t.Fatalf("redaction marker missing: message=%q details=%q api_key_details=%q", apiErr.Message, detailsText, apiKeyDetailsText)
	}
	if stderr.Len() != 0 {
		t.Fatalf("debug stderr = %q, want empty when debug disabled", stderr.String())
	}
}

func factoryCommand(debug bool, stderr *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.PersistentFlags().Bool("debug", false, "")
	if debug {
		if err := cmd.PersistentFlags().Set("debug", "true"); err != nil {
			panic(err)
		}
	}
	cmd.SetErr(stderr)
	return cmd
}

func factoryRuntime(serverURL string) config.Runtime {
	return config.Runtime{APIBaseURL: serverURL + "/v1", Locale: "de"}
}

func factoryCredential() auth.Credential {
	return factoryCredentialWithKey("ak_factory|super-secret")
}

func factoryCredentialWithKey(apiKey string) auth.Credential {
	return auth.Credential{APIKey: apiKey}
}

func writeFactoryJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", "req-factory")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
