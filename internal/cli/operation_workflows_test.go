package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/operations"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestOperationSchemaCommandUsesEmbeddedContractOffline(t *testing.T) {
	state := testutil.NewState(t)
	result := testutil.RunCommandWith(t, testutil.Options{LookupEnv: testutil.HermeticEnv(nil)},
		state.Args("operations", "schema", "search.web", "--json")...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("schema result = %#v", result)
	}
	var body struct {
		OperationID         string          `json:"operation_id"`
		Availability        string          `json:"availability"`
		IdempotencyRequired bool            `json:"idempotency_required"`
		RequestSchema       json.RawMessage `json:"request_schema"`
		ResultSchema        json.RawMessage `json:"result_schema"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &body); err != nil {
		t.Fatalf("schema JSON error = %v: %s", err, result.Stdout)
	}
	if body.OperationID != "search.web" || body.Availability != "preview" || !body.IdempotencyRequired ||
		len(body.RequestSchema) == 0 || len(body.ResultSchema) == 0 {
		t.Fatalf("schema body = %#v", body)
	}
}

func TestTypedMarkedServiceUnavailableCannotResumeAsNewWork(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	starts := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/search/web":
			starts++
			w.Header().Set("X-Chab-Idempotency-Outcome", "released")
			testutil.WriteJSON(t, w, http.StatusServiceUnavailable, testutil.ErrorEnvelope("provider_unavailable", "search is unavailable", "req-typed-released", nil))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	args := state.APIArgs(server, "search", "web", "--query", "docs", "--engine", "google", "--location", `{"country":"US"}`, "--language", "en-US", "--idempotency-key", "idem-typed-released-503", "--yes", "--json")
	first := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	if first.ExitCode == cli.ExitSuccess || starts != 1 || !strings.Contains(first.Stdout, `"state": "denied"`) || !strings.Contains(first.Stdout, `"can_resume": false`) {
		t.Fatalf("marked typed 503 = %#v, starts=%d", first, starts)
	}
	second := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	if second.ExitCode == cli.ExitSuccess || starts != 1 {
		t.Fatalf("released typed action replayed: %#v, starts=%d", second, starts)
	}
}

func TestTypedUnknownReplayUnmarkedScopeDenialStaysUnknown(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	starts := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/search/web":
			starts++
			if starts == 1 {
				testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("server_error", "uncertain", "req-first", nil))
			} else {
				testutil.WriteJSON(t, w, http.StatusForbidden, testutil.ErrorEnvelope("api_scope_missing", "scope changed", "req-replay", nil))
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	args := state.APIArgs(server, "search", "web", "--query", "docs", "--engine", "google", "--location", `{"country":"US"}`, "--language", "en-US", "--idempotency-key", "idem-typed-unknown-replay", "--yes", "--json")
	first := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	second := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	if first.ExitCode == cli.ExitSuccess || second.ExitCode == cli.ExitSuccess || starts != 2 || !strings.Contains(second.Stdout, `"state": "unknown"`) || !strings.Contains(second.Stdout, `"can_resume": true`) {
		t.Fatalf("typed unknown replay = first %#v, second %#v, starts=%d", first, second, starts)
	}
}

func TestTypedUnknownReplayRateLimitIsOnePhysicalRequest(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	starts := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/search/web":
			starts++
			if starts == 1 {
				testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("server_error", "uncertain", "req-first", nil))
			} else {
				w.Header().Set("Retry-After", "1")
				testutil.WriteJSON(t, w, http.StatusTooManyRequests, testutil.ErrorEnvelope("rate_limited", "later", "req-replay", nil))
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	args := state.APIArgs(server, "search", "web", "--query", "docs", "--engine", "google", "--location", `{"country":"US"}`, "--language", "en-US", "--idempotency-key", "idem-typed-rate-once", "--yes", "--json")
	first := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	second := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	if first.ExitCode == cli.ExitSuccess || second.ExitCode == cli.ExitSuccess || starts != 2 || !strings.Contains(second.Stdout, `"state": "unknown"`) {
		t.Fatalf("typed replay 429 = first %#v, second %#v, starts=%d", first, second, starts)
	}
}

func TestTypedUnknownReplayReleased503BecomesDeniedWithoutThirdSend(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	starts := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/search/web":
			starts++
			if starts == 1 {
				testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("server_error", "uncertain", "req-first", nil))
			} else {
				w.Header().Set("X-Chab-Idempotency-Outcome", "released")
				testutil.WriteJSON(t, w, http.StatusServiceUnavailable, testutil.ErrorEnvelope("provider_unavailable", "released", "req-replay", nil))
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	args := state.APIArgs(server, "search", "web", "--query", "docs", "--engine", "google", "--location", `{"country":"US"}`, "--language", "en-US", "--idempotency-key", "idem-typed-released-replay", "--yes", "--json")
	first := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	second := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	third := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	if first.ExitCode == cli.ExitSuccess || second.ExitCode == cli.ExitSuccess || third.ExitCode == cli.ExitSuccess || starts != 2 || !strings.Contains(second.Stdout, `"state": "denied"`) || !strings.Contains(second.Stdout, `"can_resume": false`) {
		t.Fatalf("typed released replay = first %#v, second %#v, third %#v, starts=%d", first, second, third, starts)
	}
}

func TestOperationSchemaCommandIncludesReferencedDefinitions(t *testing.T) {
	state := testutil.NewState(t)
	result := testutil.RunCommandWith(t, testutil.Options{LookupEnv: testutil.HermeticEnv(nil)},
		state.Args("operations", "schema", "scrape.dom", "--json")...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("schema result = %#v", result)
	}
	var body struct {
		ReferencedSchemas map[string]json.RawMessage `json:"referenced_schemas"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &body); err != nil {
		t.Fatalf("schema JSON error = %v: %s", err, result.Stdout)
	}
	if len(body.ReferencedSchemas["ScrapeSelectorDefinition"]) == 0 {
		t.Fatalf("schema output missing referenced ScrapeSelectorDefinition: %s", result.Stdout)
	}
}

func TestNativeOperationStartCreatesRecoverableAction(t *testing.T) {
	state, key := configuredChabAuthState(t)
	actionDir := filepath.Join(filepath.Dir(state.ConfigPath), "actions")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/search/web":
			if r.Method != http.MethodPost {
				t.Fatalf("request = %s %s", r.Method, r.URL.Path)
			}
			entries, err := os.ReadDir(actionDir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("action record before POST = %d, %v", len(entries), err)
			}
			w.Header().Set("X-RateLimit-Remaining", "0")
			fmt.Fprint(w, `{"operation":{"id":"op_search_123","status":"queued","operation_key":"search.web","family":"search","result_available":false},"meta":{"request_id":"req-start","credits":{"estimated":2,"reserved":2,"currency":"credit"}}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"search", "web",
		"--query", "docs",
		"--engine", "google",
		"--location", `{"country":"US"}`,
		"--language", "en-US",
		"--yes",
		"--json",
	)...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("start result = %#v", result)
	}

	server.AssertBearer(t, key)
	server.AssertNoIdempotencyKey(t, 0)
	server.AssertIdempotencyKeyMatches(t, 1, chabIdempotencyPattern())
	server.AssertNoIdempotencyKeyLeak(t, 1, result.Stdout)
	if got, want := server.BodyString(t, 1), `{"engine":"google","language":"en-US","location":{"country":"US"},"query":"docs"}`; got != want {
		t.Fatalf("request body = %s, want %s", got, want)
	}

	var out struct {
		Action *struct {
			ID             string `json:"id"`
			OperationKey   string `json:"operation_key"`
			EncoderVersion string `json:"encoder_version"`
			State          string `json:"state"`
		} `json:"action"`
		OperationID   string `json:"operation_id"`
		LocalRecovery struct {
			State       string `json:"state"`
			CanResume   bool   `json:"can_resume"`
			KnownRemote bool   `json:"known_remote"`
		} `json:"local_recovery"`
		LocalPersistence struct {
			State string `json:"state"`
		} `json:"local_persistence"`
		RequestID string `json:"request_id"`
		Meta      struct {
			RateLimit struct {
				Remaining *int64 `json:"remaining"`
			} `json:"rate_limit"`
			APIMeta map[string]json.RawMessage `json:"api_meta"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		t.Fatalf("start JSON error = %v: %s", err, result.Stdout)
	}
	if out.Action == nil || out.Action.ID == "" || out.Action.OperationKey != "search.web" ||
		out.Action.EncoderVersion != "chab-json-v1" || out.Action.State != "accepted" ||
		out.OperationID != "op_search_123" || out.LocalRecovery.State != "accepted" ||
		!out.LocalRecovery.CanResume || !out.LocalRecovery.KnownRemote ||
		out.LocalPersistence.State != "persisted" || out.RequestID != "req-start" {
		t.Fatalf("start output = %#v", out)
	}
	var credits struct {
		Estimated int    `json:"estimated"`
		Reserved  int    `json:"reserved"`
		Currency  string `json:"currency"`
	}
	if err := json.Unmarshal(out.Meta.APIMeta["credits"], &credits); err != nil {
		t.Fatalf("credits metadata JSON error = %v: %s", err, out.Meta.APIMeta["credits"])
	}
	if out.Meta.RateLimit.Remaining == nil || *out.Meta.RateLimit.Remaining != 0 ||
		credits.Estimated != 2 || credits.Reserved != 2 || credits.Currency != "credit" {
		t.Fatalf("start metadata = %#v", out.Meta)
	}

	rawRecord := readOnlyActionRecord(t, actionDir)
	for _, forbidden := range []string{`"query"`, "docs", key} {
		if strings.Contains(rawRecord, forbidden) {
			t.Fatalf("action record leaked %q: %s", forbidden, rawRecord)
		}
	}

	list := testutil.RunCommandWith(t, chabAuthOptions(), state.Args("operations", "actions", "list", "--json")...)
	if list.ExitCode != cli.ExitSuccess || list.Err != nil || list.Stderr != "" {
		t.Fatalf("actions list result = %#v", list)
	}
	if !strings.Contains(list.Stdout, out.Action.ID) || strings.Contains(list.Stdout, "idempotency_key") || strings.Contains(list.Stdout, "request_sha256") {
		t.Fatalf("public action list projection leaked private fields or missed action:\n%s", list.Stdout)
	}
}

func TestNativeExampleEchoSimulatedAsyncIsRecoverable(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	actionDir := filepath.Join(filepath.Dir(state.ConfigPath), "actions")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/examples/echo":
			if got, want := r.Method, http.MethodPost; got != want {
				t.Fatalf("request method = %s, want %s", got, want)
			}
			fmt.Fprint(w, `{"operation":{"id":"op_echo_async_123","status":"queued","operation_key":"examples.echo","family":"examples","result_available":false},"meta":{"request_id":"req-echo-async"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	args := state.APIArgs(server,
		"examples", "echo",
		"--simulate-async",
		"--set", `data={"hello":"world"}`,
		"--idempotency-key", "idem-echo-async-123",
		"--json",
	)
	result := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("echo async result = %#v", result)
	}
	if got, want := server.BodyString(t, 1), `{"data":{"hello":"world"},"simulate_async":true}`; got != want {
		t.Fatalf("echo async body = %s, want %s", got, want)
	}
	var out struct {
		Action struct {
			State      string `json:"state"`
			LastStatus string `json:"last_status"`
		} `json:"action"`
		OperationID   string `json:"operation_id"`
		LocalRecovery struct {
			State      string `json:"state"`
			CanResume  bool   `json:"can_resume"`
			ResumeHint string `json:"resume_hint"`
		} `json:"local_recovery"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		t.Fatalf("echo async JSON error = %v: %s", err, result.Stdout)
	}
	if out.Action.State != "accepted" || out.Action.LastStatus != "queued" ||
		out.OperationID != "op_echo_async_123" ||
		out.LocalRecovery.State != "accepted" ||
		!out.LocalRecovery.CanResume ||
		out.LocalRecovery.ResumeHint != "chab operations wait op_echo_async_123" {
		t.Fatalf("echo async output = %#v", out)
	}
	rawRecord := readOnlyActionRecord(t, actionDir)
	if !strings.Contains(rawRecord, `"state": "accepted"`) || !strings.Contains(rawRecord, `"accepted_operation_id": "op_echo_async_123"`) {
		t.Fatalf("echo async action record was not recoverable:\n%s", rawRecord)
	}

	second := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	if second.ExitCode != cli.ExitSuccess || second.Err != nil || second.Stderr != "" {
		t.Fatalf("duplicate echo async result = %#v", second)
	}
	if server.Count() != 3 || server.Request(t, 2).Path != "/v1/me" {
		t.Fatalf("accepted echo replayed effectful request; requests=%#v", server.Requests())
	}
}

func TestNativeOperationStartWaitPersistsTerminalStatus(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	actionDir := filepath.Join(filepath.Dir(state.ConfigPath), "actions")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/search/web":
			fmt.Fprint(w, `{"operation":{"id":"op_wait_123","status":"queued","operation_key":"search.web","family":"search","result_available":false},"meta":{"request_id":"req-start","credits":{"reserved":7,"currency":"credit"}}}`)
		case "/v1/operations/op_wait_123":
			if got := r.URL.Query().Get("wait"); got != "25s" {
				t.Fatalf("wait query = %q, want 25s on %s", got, r.URL.RawQuery)
			}
			w.Header().Set("X-RateLimit-Remaining", "4")
			fmt.Fprint(w, `{"operation":{"id":"op_wait_123","status":"succeeded","operation_key":"search.web","family":"search","result_available":true},"meta":{"request_id":"req-wait","credits":{"charged":3,"currency":"credit"}}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"search", "web",
		"--query", "docs",
		"--engine", "google",
		"--location", `{"country":"US"}`,
		"--language", "en-US",
		"--yes",
		"--wait",
		"--json",
	)...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil {
		t.Fatalf("start --wait result = %#v", result)
	}
	if !strings.Contains(result.Stderr, "accepted operation op_wait_123") {
		t.Fatalf("stderr missing accepted operation guidance:\n%s", result.Stderr)
	}
	var out struct {
		Action struct {
			State      string `json:"state"`
			LastStatus string `json:"last_status"`
		} `json:"action"`
		OperationID      string          `json:"operation_id"`
		Server           json.RawMessage `json:"server"`
		LocalPersistence struct {
			State string `json:"state"`
		} `json:"local_persistence"`
		LocalRecovery struct {
			State       string `json:"state"`
			KnownRemote bool   `json:"known_remote"`
		} `json:"local_recovery"`
		RequestID string `json:"request_id"`
		Meta      struct {
			RateLimit struct {
				Remaining *int64 `json:"remaining"`
			} `json:"rate_limit"`
			APIMeta map[string]json.RawMessage `json:"api_meta"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		t.Fatalf("start --wait JSON error = %v: %s", err, result.Stdout)
	}
	var serverPayload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(out.Server, &serverPayload); err != nil {
		t.Fatalf("server payload JSON error = %v: %s", err, out.Server)
	}
	if out.OperationID != "op_wait_123" ||
		out.Action.State != "completed" ||
		out.Action.LastStatus != "succeeded" ||
		out.LocalRecovery.State != "completed" ||
		!out.LocalRecovery.KnownRemote ||
		out.LocalPersistence.State != "persisted" ||
		out.RequestID != "req-wait" ||
		serverPayload.Status != "succeeded" {
		t.Fatalf("start --wait output = %#v", out)
	}
	var credits struct {
		Charged  int    `json:"charged"`
		Currency string `json:"currency"`
	}
	if err := json.Unmarshal(out.Meta.APIMeta["credits"], &credits); err != nil {
		t.Fatalf("wait credits JSON error = %v: %s", err, out.Meta.APIMeta["credits"])
	}
	if out.Meta.RateLimit.Remaining == nil || *out.Meta.RateLimit.Remaining != 4 ||
		credits.Charged != 3 || credits.Currency != "credit" {
		t.Fatalf("wait metadata = %#v", out.Meta)
	}
	rawRecord := readOnlyActionRecord(t, actionDir)
	if !strings.Contains(rawRecord, `"state": "completed"`) || !strings.Contains(rawRecord, `"last_status": "succeeded"`) {
		t.Fatalf("terminal action state was not persisted:\n%s", rawRecord)
	}
}

func TestNativeOperationStartWaitHumanOutputIncludesTerminalMetadata(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/search/web":
			fmt.Fprint(w, `{"operation":{"id":"op_human_wait_123","status":"queued","operation_key":"search.web","family":"search","result_available":false},"meta":{"request_id":"req-start","credits":{"reserved":7,"currency":"credit"}}}`)
		case "/v1/operations/op_human_wait_123":
			w.Header().Set("X-RateLimit-Remaining", "4")
			fmt.Fprint(w, `{"operation":{"id":"op_human_wait_123","status":"succeeded","operation_key":"search.web","family":"search","result_available":true},"meta":{"request_id":"req-wait","credits":{"charged":3,"currency":"credit"}}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"search", "web",
		"--query", "docs",
		"--engine", "google",
		"--location", `{"country":"US"}`,
		"--language", "en-US",
		"--yes",
		"--wait",
	)...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil {
		t.Fatalf("human start --wait result = %#v", result)
	}
	for _, want := range []string{"Request ID: req-wait", "Rate limit remaining: 4", "Credits: {\"charged\":3,\"currency\":\"credit\"}"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("human output missing %q:\n%s", want, result.Stdout)
		}
	}
	if strings.Contains(result.Stdout, "req-start") || strings.Contains(result.Stdout, `"reserved":7`) {
		t.Fatalf("human output kept stale submission metadata:\n%s", result.Stdout)
	}
}

func TestNativeOperationStartWaitReturnsNonzeroForTerminalFailure(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/search/web":
			fmt.Fprint(w, `{"operation":{"id":"op_failed_123","status":"queued","operation_key":"search.web","family":"search","result_available":false},"meta":{"request_id":"req-start"}}`)
		case "/v1/operations/op_failed_123":
			fmt.Fprint(w, `{"operation":{"id":"op_failed_123","status":"failed","operation_key":"search.web","family":"search","result_available":false},"meta":{"request_id":"req-wait"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"search", "web",
		"--query", "docs",
		"--engine", "google",
		"--location", `{"country":"US"}`,
		"--language", "en-US",
		"--yes",
		"--wait",
		"--json",
	)...)
	if result.ExitCode != cli.ExitUsage || result.Err == nil || result.Stdout == "" {
		t.Fatalf("start --wait failed-terminal result = %#v", result)
	}
	if !strings.Contains(result.Stderr, "finished with status failed") {
		t.Fatalf("stderr missing terminal failure:\n%s", result.Stderr)
	}
	if !strings.Contains(result.Stdout, `"last_status": "failed"`) {
		t.Fatalf("stdout missing persisted failed status:\n%s", result.Stdout)
	}
}

func TestStartWaitEmitsAcceptedOperationBeforePollingReturns(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	pollStarted := make(chan struct{})
	releasePoll := make(chan struct{})
	var once sync.Once
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/search/web":
			fmt.Fprint(w, `{"operation":{"id":"op_early_123","status":"queued","operation_key":"search.web","family":"search","result_available":false},"meta":{"request_id":"req-start"}}`)
		case "/v1/operations/op_early_123":
			once.Do(func() { close(pollStarted) })
			select {
			case <-releasePoll:
			case <-r.Context().Done():
				return
			}
			fmt.Fprint(w, `{"operation":{"id":"op_early_123","status":"succeeded","operation_key":"search.web","family":"search","result_available":true},"meta":{"request_id":"req-wait"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	stdout := &safeBuffer{}
	stderr := &safeBuffer{}
	done := make(chan struct {
		code int
		err  error
	}, 1)
	go func() {
		code, err := cli.RunWith(state.APIArgs(server,
			"search", "web",
			"--query", "docs",
			"--engine", "google",
			"--location", `{"country":"US"}`,
			"--language", "en-US",
			"--yes",
			"--wait",
			"--json",
		), stdout, stderr, cliWorkflowOptions())
		done <- struct {
			code int
			err  error
		}{code: code, err: err}
	}()

	select {
	case <-pollStarted:
	case <-time.After(2 * time.Second):
		close(releasePoll)
		t.Fatal("polling did not start")
	}
	if !strings.Contains(stderr.String(), "accepted operation op_early_123") {
		close(releasePoll)
		t.Fatalf("stderr before poll release missing accepted operation id:\n%s", stderr.String())
	}
	close(releasePoll)
	outcome := <-done
	if outcome.code != cli.ExitSuccess || outcome.err != nil {
		t.Fatalf("start --wait result code=%d err=%v stdout=%s stderr=%s", outcome.code, outcome.err, stdout.String(), stderr.String())
	}
}

func TestStartWaitInterruptedWritesRecoveryOutput(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	ctx, cancel := context.WithCancel(context.Background())
	pollStarted := make(chan struct{})
	var once sync.Once
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/search/web":
			fmt.Fprint(w, `{"operation":{"id":"op_interrupt_123","status":"queued","operation_key":"search.web","family":"search","result_available":false},"meta":{"request_id":"req-start"}}`)
		case "/v1/operations/op_interrupt_123":
			once.Do(func() { close(pollStarted) })
			<-r.Context().Done()
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	stdout := &safeBuffer{}
	stderr := &safeBuffer{}
	done := make(chan struct {
		code int
		err  error
	}, 1)
	opts := cliWorkflowOptions()
	opts.Context = ctx
	go func() {
		code, err := cli.RunWith(state.APIArgs(server,
			"search", "web",
			"--query", "docs",
			"--engine", "google",
			"--location", `{"country":"US"}`,
			"--language", "en-US",
			"--yes",
			"--wait",
			"--json",
		), stdout, stderr, opts)
		done <- struct {
			code int
			err  error
		}{code: code, err: err}
	}()

	select {
	case <-pollStarted:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("polling did not start")
	}
	cancel()
	select {
	case outcome := <-done:
		if outcome.code == cli.ExitSuccess || outcome.err == nil {
			t.Fatalf("interrupted wait unexpectedly succeeded: code=%d err=%v stdout=%s stderr=%s", outcome.code, outcome.err, stdout.String(), stderr.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("interrupted wait did not return")
	}
	if !strings.Contains(stdout.String(), `"operation_id": "op_interrupt_123"`) ||
		!strings.Contains(stdout.String(), `"resume_hint": "chab operations wait op_interrupt_123"`) ||
		!strings.Contains(stdout.String(), `"can_resume": true`) {
		t.Fatalf("stdout missing recovery object:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "stopped waiting for operation op_interrupt_123") {
		t.Fatalf("stderr missing interrupt guidance:\n%s", stderr.String())
	}
}

func TestOperationWaitEnforcesShortDeadline(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/operations/op_slow_123":
			if got := r.URL.Query().Get("wait"); got != "" {
				t.Fatalf("wait query = %q, want no long-poll for a sub-headroom timeout", got)
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(120 * time.Millisecond):
				fmt.Fprint(w, `{"operation":{"id":"op_slow_123","status":"succeeded","operation_key":"search.web","family":"search","result_available":true},"meta":{"request_id":"req-late"}}`)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	started := time.Now()
	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"operations", "wait", "op_slow_123",
		"--timeout", "10ms",
		"--json",
	)...)
	if result.ExitCode == cli.ExitSuccess || result.Err == nil || result.Stdout == "" {
		t.Fatalf("wait result = %#v, want timeout failure with recovery stdout", result)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("wait ignored request deadline; elapsed=%s result=%#v", elapsed, result)
	}
	if !strings.Contains(result.Stderr, "timed out waiting for operation op_slow_123") {
		t.Fatalf("stderr missing timeout recovery guidance:\n%s", result.Stderr)
	}
	if !strings.Contains(result.Stdout, `"operation_id": "op_slow_123"`) ||
		!strings.Contains(result.Stdout, `"state": "timeout"`) ||
		!strings.Contains(result.Stdout, `"resume_hint": "chab operations wait op_slow_123"`) {
		t.Fatalf("stdout missing timeout recovery object:\n%s", result.Stdout)
	}
}

func TestOperationBulkCancelSynchronousSummaryCompletes(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/operations/cancel":
			if got, want := r.Method, http.MethodPost; got != want {
				t.Fatalf("request method = %s, want %s", got, want)
			}
			fmt.Fprint(w, `{"data":{"cancelled":[{"id":"op_cancelled","status":"cancelling","operation_key":"examples.echo","family":"examples","result_available":false}],"already_terminal":[],"unsupported":[],"not_found_or_not_authorized":0,"counts":{"cancelled":1,"already_terminal":0,"unsupported":0,"not_found_or_not_authorized":0},"capped":false},"meta":{"request_id":"req-bulk-cancel"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"operations", "bulk-cancel",
		"--input", `{"status":"running"}`,
		"--yes",
		"--json",
	)...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("bulk cancel result = %#v", result)
	}
	var out struct {
		Action struct {
			State      string `json:"state"`
			LastStatus string `json:"last_status"`
		} `json:"action"`
		OperationID   string `json:"operation_id"`
		LocalRecovery struct {
			State     string `json:"state"`
			CanResume bool   `json:"can_resume"`
		} `json:"local_recovery"`
		Server struct {
			Counts struct {
				Cancelled int `json:"cancelled"`
			} `json:"counts"`
		} `json:"server"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		t.Fatalf("bulk cancel JSON error = %v: %s", err, result.Stdout)
	}
	if out.Action.State != "completed" || out.Action.LastStatus != "" ||
		out.OperationID != "" || out.LocalRecovery.State != "completed" ||
		out.LocalRecovery.CanResume || out.Server.Counts.Cancelled != 1 {
		t.Fatalf("bulk cancel output = %#v", out)
	}
}

func TestOperationResumeReplaysNoBodyCancelToRecordedPath(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	operationID := "op/needs?escape"
	cancelPath := "/v1/" + api.Path("operations", operationID, "cancel")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/operations/op%2Fneeds%3Fescape/cancel":
			fmt.Fprint(w, `{"operation":{"id":"op_cancel_123","status":"cancelling","operation_key":"examples.echo","family":"examples","result_available":false},"meta":{"request_id":"req-cancel-resume"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	actionID := prepareJournalAction(t, state, server.APIBaseURL(), "operations.cancel", cancelPath, nil, "idem-cancel-123")
	store := operations.StoreForRuntime(config.Runtime{ConfigPath: state.ConfigPath}, time.Now)
	if err := store.MarkUnknown(actionID, errors.New("connection closed before response")); err != nil {
		t.Fatalf("MarkUnknown() error = %v", err)
	}

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"operations", "resume", actionID,
		"--json",
	)...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("resume cancel result = %#v", result)
	}
	server.AssertNoBody(t, 1)
	if got := server.Request(t, 1).Path; got != "/v1/operations/op%2Fneeds%3Fescape/cancel" {
		t.Fatalf("resume cancel path = %s", got)
	}
	var out struct {
		Action struct {
			ID    string `json:"id"`
			State string `json:"state"`
			Path  string `json:"path"`
		} `json:"action"`
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		t.Fatalf("resume JSON error = %v: %s", err, result.Stdout)
	}
	if out.Action.ID != actionID || out.Action.State != "accepted" || out.Action.Path != cancelPath || out.OperationID != "op_cancel_123" {
		t.Fatalf("resume output = %#v", out)
	}
	assertActionFileCount(t, filepath.Join(filepath.Dir(state.ConfigPath), "actions"), 1)
}

func TestOperationCancelEscapesOperationIDPath(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	operationID := "op/needs?escape"
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/operations/op%2Fneeds%3Fescape/cancel":
			fmt.Fprint(w, `{"operation":{"id":"op_cancel_123","status":"cancelling","operation_key":"examples.echo","family":"examples","result_available":false},"meta":{"request_id":"req-cancel"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.RequestURI())
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"operations", "cancel", operationID,
		"--yes",
		"--json",
	)...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("cancel result = %#v", result)
	}
	server.AssertNoBody(t, 1)
	if got := server.Request(t, 1).Path; got != "/v1/operations/op%2Fneeds%3Fescape/cancel" {
		t.Fatalf("cancel path = %s", got)
	}
}

func TestOperationResumeUnknownRejectsMismatchedInputBeforeHTTP(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	actionID := prepareJournalAction(t, state, "http://api.test/v1", "search.web", "/v1/search/web", []byte(`{"engine":"google","language":"en-US","location":{"country":"US"},"query":"docs"}`), "idem-search-verify")
	store := operations.StoreForRuntime(config.Runtime{ConfigPath: state.ConfigPath}, time.Now)
	if err := store.MarkUnknown(actionID, errors.New("connection closed before response")); err != nil {
		t.Fatalf("MarkUnknown() error = %v", err)
	}

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.Args(
		"operations", "resume", actionID,
		"--input", `{"query":"changed","engine":"google","location":{"country":"US"},"language":"en-US"}`,
		"--json",
	)...)
	if result.ExitCode != cli.ExitUsage || result.Err == nil || result.Stdout != "" {
		t.Fatalf("resume mismatch result = %#v", result)
	}
	if !strings.Contains(result.Stderr, "input does not match") {
		t.Fatalf("stderr missing request-hash mismatch:\n%s", result.Stderr)
	}
}

func TestOperationResumeKnownOperationReturnsActionWrapperAndRejectsMissingStatus(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/operations/op_missing_status_123":
			fmt.Fprint(w, `{"operation":{"id":"op_missing_status_123","operation_key":"search.web","family":"search","result_available":false},"meta":{"request_id":"req-status"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	actionID := prepareJournalAction(t, state, server.APIBaseURL(), "search.web", "/v1/search/web", []byte(`{"engine":"google","language":"en-US","location":{"country":"US"},"query":"docs"}`), "idem-search-123")
	store := operations.StoreForRuntime(config.Runtime{ConfigPath: state.ConfigPath}, time.Now)
	if _, err := store.MarkAccepted(actionID, operations.OperationPayload{ID: "op_missing_status_123", Status: "queued"}, api.ResponseMeta{RequestID: "req-record"}); err != nil {
		t.Fatalf("MarkAccepted() error = %v", err)
	}

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"operations", "resume", actionID,
		"--json",
	)...)
	if result.ExitCode == cli.ExitSuccess || result.Err == nil || result.Stdout == "" {
		t.Fatalf("resume missing status result = %#v", result)
	}
	if !strings.Contains(result.Stdout, `"action"`) || !strings.Contains(result.Stdout, `"operation_id": "op_missing_status_123"`) {
		t.Fatalf("stdout missing action wrapper:\n%s", result.Stdout)
	}
	if !strings.Contains(result.Stderr, "missing status") {
		t.Fatalf("stderr missing malformed status error:\n%s", result.Stderr)
	}
}

func TestOperationResumeKnownOperationReturnsPersistenceFailure(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	actionDir := filepath.Join(filepath.Dir(state.ConfigPath), "actions")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/operations/op_persist_123":
			replaceActionRecordWithDirectory(t, actionDir)
			fmt.Fprint(w, `{"operation":{"id":"op_persist_123","status":"succeeded","operation_key":"search.web","family":"search","result_available":true},"meta":{"request_id":"req-status"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	actionID := prepareJournalAction(t, state, server.APIBaseURL(), "search.web", "/v1/search/web", []byte(`{"engine":"google","language":"en-US","location":{"country":"US"},"query":"docs"}`), "idem-search-persist")
	store := operations.StoreForRuntime(config.Runtime{ConfigPath: state.ConfigPath}, time.Now)
	if _, err := store.MarkAccepted(actionID, operations.OperationPayload{ID: "op_persist_123", Status: "queued"}, api.ResponseMeta{RequestID: "req-record"}); err != nil {
		t.Fatalf("MarkAccepted() error = %v", err)
	}

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"operations", "resume", actionID,
		"--json",
	)...)
	if result.ExitCode == cli.ExitSuccess || result.Err == nil || result.Stdout == "" {
		t.Fatalf("resume persistence failure result = %#v", result)
	}
	if !strings.Contains(result.Stdout, `"operation_id": "op_persist_123"`) ||
		!strings.Contains(result.Stdout, `"state": "failed"`) ||
		!strings.Contains(result.Stderr, "failed to persist local action state") {
		t.Fatalf("resume persistence failure output missing details:\nstdout=%s\nstderr=%s", result.Stdout, result.Stderr)
	}
}

func TestOperationResumeRejectsUnsupportedEncoderBeforeHTTP(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unsupported encoder contacted server: %s %s", r.Method, r.URL.Path)
	})
	actionID := prepareJournalActionWithEncoder(t, state, server.APIBaseURL(), "examples.echo", "/v1/examples/echo", []byte(`{"data":{}}`), "idem-unsupported-encoder", "old-json-v0")
	store := operations.StoreForRuntime(config.Runtime{ConfigPath: state.ConfigPath}, time.Now)
	if err := store.MarkUnknown(actionID, errors.New("connection closed before response")); err != nil {
		t.Fatalf("MarkUnknown() error = %v", err)
	}

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"operations", "resume", actionID,
		"--json",
	)...)
	if result.ExitCode != cli.ExitUsage || result.Err == nil || result.Stdout != "" {
		t.Fatalf("unsupported encoder result = %#v", result)
	}
	if !strings.Contains(result.Stderr, "unsupported encoder version") {
		t.Fatalf("stderr missing unsupported encoder guidance:\n%s", result.Stderr)
	}
	server.AssertNoRequests(t)
}

func TestOperationResumeRawActionRequiresByteIdenticalInput(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	var server *testutil.APIServer
	server = testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/examples/echo":
			if got, want := server.BodyString(t, 1), `{"b":2,"a":1}`; got != want {
				t.Fatalf("raw resume body = %s, want %s", got, want)
			}
			fmt.Fprint(w, `{"data":{"ok":true},"meta":{"request_id":"req-raw-resume"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	actionID := prepareJournalActionWithEncoder(t, state, server.APIBaseURL(), "examples.echo", "/v1/examples/echo", []byte(`{"b":2,"a":1}`), "idem-raw-resume", operations.RawEncoderVersion)
	store := operations.StoreForRuntime(config.Runtime{ConfigPath: state.ConfigPath}, time.Now)
	if err := store.MarkUnknown(actionID, errors.New("connection closed before response")); err != nil {
		t.Fatalf("MarkUnknown() error = %v", err)
	}

	mismatch := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"operations", "resume", actionID,
		"--input", `{"a":1,"b":2}`,
		"--json",
	)...)
	if mismatch.ExitCode != cli.ExitUsage || mismatch.Err == nil || mismatch.Stdout != "" {
		t.Fatalf("raw mismatch result = %#v", mismatch)
	}
	if !strings.Contains(mismatch.Stderr, "input does not match") {
		t.Fatalf("stderr missing raw input mismatch:\n%s", mismatch.Stderr)
	}
	if server.Count() != 0 {
		t.Fatalf("raw mismatch contacted server; requests=%#v", server.Requests())
	}

	match := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"operations", "resume", actionID,
		"--input", `{"b":2,"a":1}`,
		"--json",
	)...)
	if match.ExitCode != cli.ExitSuccess || match.Err != nil || match.Stderr != "" {
		t.Fatalf("raw match result = %#v", match)
	}
	if !strings.Contains(match.Stdout, `"state": "completed"`) {
		t.Fatalf("raw match stdout missing completed receipt:\n%s", match.Stdout)
	}
}

func TestNativeServerDryRunSkipsJournalIdempotencyAndConfirmation(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	actionDir := filepath.Join(filepath.Dir(state.ConfigPath), "actions")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/screenshots/url":
			if r.Method != http.MethodPost {
				t.Fatalf("request = %s %s", r.Method, r.URL.Path)
			}
			fmt.Fprint(w, `{"data":{"valid":true,"dry_run":true},"meta":{"request_id":"req-dry"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"screenshots", "url",
		"--input", `{"url":"https://example.test","dry_run":true}`,
		"--no-prompt",
		"--json",
	)...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("server dry-run result = %#v", result)
	}
	server.AssertNoIdempotencyKey(t, 0)
	server.AssertNoIdempotencyKey(t, 1)
	if got, want := server.BodyString(t, 1), `{"dry_run":true,"url":"https://example.test"}`; got != want {
		t.Fatalf("request body = %s, want %s", got, want)
	}
	assertNotExists(t, actionDir)

	var out struct {
		Server struct {
			Valid  bool `json:"valid"`
			DryRun bool `json:"dry_run"`
		} `json:"server"`
		LocalPersistence struct {
			State string `json:"state"`
		} `json:"local_persistence"`
		LocalRecovery struct {
			State     string `json:"state"`
			CanResume bool   `json:"can_resume"`
		} `json:"local_recovery"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		t.Fatalf("server dry-run JSON error = %v: %s", err, result.Stdout)
	}
	if !out.Server.Valid || !out.Server.DryRun ||
		out.LocalPersistence.State != "not_recorded" ||
		out.LocalRecovery.State != "server_dry_run" ||
		out.LocalRecovery.CanResume ||
		out.RequestID != "req-dry" {
		t.Fatalf("server dry-run output = %#v", out)
	}
}

func TestNativeServerDryRunRejectsUnsupportedOperationBeforeHTTP(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unsupported dry-run contacted server: %s %s", r.Method, r.URL.Path)
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"search", "web",
		"--input", `{"query":"docs","engine":"google","location":{"country":"US"},"language":"en-US","dry_run":true}`,
		"--no-prompt",
		"--json",
	)...)
	if result.ExitCode != cli.ExitUsage || result.Stdout != "" || result.Err == nil {
		t.Fatalf("unsupported server dry-run result = %#v", result)
	}
	if !strings.Contains(result.Stderr, "does not support server dry runs") {
		t.Fatalf("stderr missing unsupported dry-run guidance:\n%s", result.Stderr)
	}
	server.AssertNoRequests(t)
}

func TestOperationBudgetRulesFailBeforeHTTP(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("operation command contacted server after local budget failure: %s %s", r.Method, r.URL.Path)
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"search", "web",
		"--query", "docs",
		"--engine", "google",
		"--location", `{"country":"US"}`,
		"--language", "en-US",
		"--max-credits", "3",
		"--yes",
		"--json",
	)...)
	if result.ExitCode != cli.ExitUsage || result.Stdout != "" || result.Err == nil {
		t.Fatalf("result = %#v, want local usage failure", result)
	}
	if !strings.Contains(result.Stderr, "--max-credits is supported only") {
		t.Fatalf("stderr missing max-credits guidance:\n%s", result.Stderr)
	}
	server.AssertNoRequests(t)
}

func TestNativeExampleRequestsValidateAgainstEmbeddedSchemas(t *testing.T) {
	state := testutil.NewState(t)
	tests := [][]string{
		{"business", "details", "--business-ref", "bref_EXAMPLE", "--engine", "google", "--language", "en-US", "--dry-run", "--json"},
		{"scrape", "dom", "--url", "https://example.test", "--selectors", `{"title":{"selector":"h1"}}`, "--dry-run", "--json"},
		{"convert", "file", "--source", `{"type":"file_id","id":"file_123"}`, "--output-format", "markdown", "--dry-run", "--json"},
		{"translate", "text-or-document", "--source", `{"type":"inline","format":"text","content":"Hallo"}`, "--target-language", "en", "--max-credits", "25", "--dry-run", "--json"},
		{"llm", "generate", "--model", "fast-chat", "--prompt", "Write a haiku", "--max-credits", "20", "--dry-run", "--json"},
		{"llm", "embeddings", "--model", "embedding-small", "--input-json", `["hello"]`, "--max-credits", "10", "--dry-run", "--json"},
		{"research", "deep", "--question", "Market size?", "--mode", "standard", "--source-budget", "8", "--max-credits", "100", "--dry-run", "--json"},
	}
	for _, args := range tests {
		args := args
		t.Run(strings.Join(args[:2], " "), func(t *testing.T) {
			result := testutil.RunCommandWith(t, testutil.Options{LookupEnv: testutil.HermeticEnv(nil)}, state.Args(args...)...)
			if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
				t.Fatalf("dry-run result = %#v", result)
			}
			if !strings.Contains(result.Stdout, `"request"`) {
				t.Fatalf("dry-run stdout missing request preview:\n%s", result.Stdout)
			}
		})
	}
}

func TestNativeRequestValidationRejectsOldInvalidExamplesBeforeHTTP(t *testing.T) {
	state := testutil.NewState(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "convert source", args: []string{"convert", "file", "--source", `{"file_id":"file_123"}`, "--output-format", "markdown", "--dry-run", "--json"}, want: "missing required field type"},
		{name: "dom selectors", args: []string{"scrape", "dom", "--url", "https://example.test", "--selectors", `{"title":"h1"}`, "--dry-run", "--json"}, want: "must match exactly one allowed shape"},
		{name: "dom raw html flag", args: []string{"scrape", "dom", "--url", "https://example.test", "--selectors", `{"title":{"selector":"h1"}}`, "--include-raw-html", "--dry-run", "--json"}, want: "unknown flag: --include-raw-html"},
		{name: "translate source", args: []string{"translate", "text-or-document", "--source", `{"text":"Hallo"}`, "--target-language", "en", "--max-credits", "25", "--dry-run", "--json"}, want: "missing required field type"},
		{name: "llm model", args: []string{"llm", "generate", "--model", "gpt-5", "--prompt", "Write a haiku", "--max-credits", "20", "--dry-run", "--json"}, want: "allowed values"},
		{name: "research mode", args: []string{"research", "deep", "--question", "Market size?", "--mode", "web", "--source-budget", "8", "--max-credits", "100", "--dry-run", "--json"}, want: "allowed values"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := testutil.RunCommandWith(t, testutil.Options{LookupEnv: testutil.HermeticEnv(nil)}, state.Args(tc.args...)...)
			if result.ExitCode != cli.ExitUsage || result.Err == nil || result.Stdout != "" {
				t.Fatalf("dry-run result = %#v, want local schema failure", result)
			}
			if !strings.Contains(result.Stderr, tc.want) {
				t.Fatalf("stderr missing %q:\n%s", tc.want, result.Stderr)
			}
		})
	}
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func cliWorkflowOptions() cli.Options {
	return cli.Options{
		Stdin:     bytes.NewReader(nil),
		LookupEnv: testutil.HermeticEnv(nil),
		BootstrapFactory: func(rt config.Runtime, cmd *cobra.Command) (cmdutil.BootstrapClient, error) {
			return api.NewBootstrap(api.OptionsForBootstrap(rt, "1.0.0", false, cmd.ErrOrStderr()))
		},
	}
}

func prepareJournalAction(t *testing.T, state testutil.State, apiBaseURL, operationKey, path string, request []byte, idem string) string {
	t.Helper()
	return prepareJournalActionWithEncoder(t, state, apiBaseURL, operationKey, path, request, idem, "")
}

func prepareJournalActionWithEncoder(t *testing.T, state testutil.State, apiBaseURL, operationKey, path string, request []byte, idem string, encoder string) string {
	t.Helper()
	op, ok := chabcontract.MustLoad().Find(operationKey)
	if !ok {
		t.Fatalf("operation %s missing from registry", operationKey)
	}
	prepared, err := operations.StoreForRuntime(config.Runtime{
		Profile:    "local",
		ConfigPath: state.ConfigPath,
		APIBaseURL: apiBaseURL,
	}, time.Now).Prepare(operations.PrepareInput{
		Profile:        "local",
		Destination:    apiBaseURL,
		TokenPublicID:  "ak_01HY0000000000000000000000",
		RequiredScope:  op.RequiredScope,
		OperationKey:   operationKey,
		Method:         op.Method,
		Path:           path,
		EncoderVersion: encoder,
		RequestBytes:   request,
		IdempotencyKey: idem,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	return prepared.Record.ID
}

func replaceActionRecordWithDirectory(t *testing.T, actionDir string) {
	t.Helper()
	entries, err := os.ReadDir(actionDir)
	if err != nil {
		t.Fatalf("read action dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(actionDir, entry.Name())
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove action record: %v", err)
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("replace action record with directory: %v", err)
		}
		return
	}
	t.Fatalf("no action record found in %s", actionDir)
}

func assertActionFileCount(t *testing.T, actionDir string, want int) {
	t.Helper()
	entries, err := os.ReadDir(actionDir)
	if err != nil {
		t.Fatalf("read action dir: %v", err)
	}
	if len(entries) != want {
		t.Fatalf("action files = %v, want %d", entryNames(entries), want)
	}
}

func readOnlyActionRecord(t *testing.T, actionDir string) string {
	t.Helper()
	entries, err := os.ReadDir(actionDir)
	if err != nil {
		t.Fatalf("read action dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("action files = %v, want one", entryNames(entries))
	}
	raw, err := os.ReadFile(filepath.Join(actionDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read action record: %v", err)
	}
	return string(raw)
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func chabIdempotencyPattern() *regexp.Regexp {
	return regexp.MustCompile(`^chab-[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
}
