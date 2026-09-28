package api_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/config"
)

const fakeAPIKey = "ak_test|super-secret"

var fixedNow = time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)

type fakeSleeper struct {
	waits []time.Duration
	err   error
}

func (s *fakeSleeper) Sleep(ctx context.Context, d time.Duration) error {
	if s.err != nil {
		return s.err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.waits = append(s.waits, d)
	return nil
}

func newTestClient(t *testing.T, baseURL string, configure func(*api.Options)) (*api.Client, *bytes.Buffer, *fakeSleeper) {
	t.Helper()

	debug := &bytes.Buffer{}
	sleeper := &fakeSleeper{}
	opts := api.OptionsFromRuntime(
		config.Runtime{APIBaseURL: baseURL, Locale: "en"},
		auth.Credential{APIKey: fakeAPIKey},
		"1.2.3",
		true,
		debug,
	)
	opts.Sleeper = sleeper
	opts.Now = func() time.Time { return fixedNow }
	opts.BackoffJitter = func(d time.Duration) time.Duration { return d }
	if configure != nil {
		configure(&opts)
	}

	client, err := api.New(opts)
	if err != nil {
		t.Fatalf("api.New() error = %v", err)
	}
	return client, debug, sleeper
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", "req-header")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
