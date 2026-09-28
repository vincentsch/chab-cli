package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestRawAPIRefusesChabOneTimeSecretRoutes(t *testing.T) {
	for _, tc := range []struct {
		method string
		route  string
		args   []string
	}{
		{method: "post", route: "/tokens", args: []string{"--field", "name=demo"}},
		{method: "post", route: "/webhooks/endpoints", args: []string{"--field", "name=demo"}},
		{method: "post", route: "/webhooks/endpoints/endpoint_123/rotate-secret", args: []string{"--field", "name=demo"}},
		{method: "get", route: "/management-approvals/map_review"},
	} {
		t.Run(tc.method+" "+tc.route, func(t *testing.T) {
			state, _ := configuredChabAuthState(t)
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("raw API contacted server for %s %s", r.Method, r.URL.Path)
			})

			args := []string{"api", tc.method, tc.route}
			args = append(args, tc.args...)
			args = append(args, "--json")
			result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server, args...)...)
			if result.ExitCode != cli.ExitUsage || result.Err == nil {
				t.Fatalf("result = %#v, want usage refusal", result)
			}
			if !strings.Contains(result.Stderr, "one-time plaintext secrets or management proofs") || !strings.Contains(result.Stderr, "private-output") {
				t.Fatalf("stderr missing guidance:\n%s", result.Stderr)
			}
			server.AssertNoRequests(t)
		})
	}
}

func TestRawAPISuccessRedactsRequestSecrets(t *testing.T) {
	state, key := configuredChabAuthState(t)
	const pin = "123456"
	const idem = "idem-secret"
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/examples/echo":
			if r.Method != http.MethodPost {
				t.Fatalf("request = %s %s", r.Method, r.URL.Path)
			}
			fmt.Fprintf(w, `{"data":{"pin":123456,"idempotency":"%s","credential":"%s","kept":"ok"},"meta":{"request_id":"req-raw"}}`, idem, key)
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"api", "post", "/examples/echo",
		"--body", `{"pin":123456,"event":"demo"}`,
		"--secret-field", "pin",
		"--idempotency-key", idem,
		"--json",
	)...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("result = %#v", result)
	}
	for _, forbidden := range []string{pin, idem, key} {
		if strings.Contains(result.Stdout, forbidden) {
			t.Fatalf("stdout leaked request secret %q:\n%s", forbidden, result.Stdout)
		}
	}
	for _, want := range []string{`"pin": "[REDACTED]"`, `"idempotency": "[REDACTED]"`, `"credential": "[REDACTED]"`, `"kept": "ok"`} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, result.Stdout)
		}
	}
	var out struct {
		Action *struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"action"`
		Server struct {
			Kept string `json:"kept"`
		} `json:"server"`
		LocalRecovery struct {
			State string `json:"state"`
		} `json:"local_recovery"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		t.Fatalf("raw action JSON error = %v: %s", err, result.Stdout)
	}
	if out.Action == nil || out.Action.State != "completed" || out.Server.Kept != "ok" || out.LocalRecovery.State != "completed" || out.RequestID != "req-raw" {
		t.Fatalf("raw action output = %#v", out)
	}
	server.AssertBearer(t, key)
	server.AssertNoIdempotencyKey(t, 0)
	server.AssertIdempotencyKey(t, 1, idem)
	assertRawActionRecord(t, state, idem, key)

	second := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"api", "post", "/examples/echo",
		"--body", `{"pin":123456,"event":"demo"}`,
		"--secret-field", "pin",
		"--idempotency-key", idem,
		"--json",
	)...)
	if second.ExitCode != cli.ExitSuccess || second.Err != nil || second.Stderr != "" {
		t.Fatalf("duplicate raw action result = %#v", second)
	}
	if server.Count() != 3 || server.Request(t, 2).Path != "/v1/me" {
		t.Fatalf("completed raw action replayed effectful request; requests=%#v", server.Requests())
	}
	if !strings.Contains(second.Stdout, `"state": "completed"`) || strings.Contains(second.Stdout, idem) {
		t.Fatalf("duplicate output missing completed recovery or leaked key:\n%s", second.Stdout)
	}
}

func TestRawAPIPaidOperationRequiresAcknowledgementBeforeMutation(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("raw API contacted server for unacknowledged paid operation: %s %s", r.Method, r.URL.Path)
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"api", "post", "/search/web",
		"--body", `{"query":"docs","engine":"google","location":{"country":"US"},"language":"en-US"}`,
		"--no-prompt",
		"--json",
	)...)
	if result.ExitCode != cli.ExitUsage || result.Stdout != "" || result.Err == nil {
		t.Fatalf("result = %#v, want local confirmation refusal", result)
	}
	if !strings.Contains(result.Stderr, "confirmation") || !strings.Contains(result.Stderr, "--yes") {
		t.Fatalf("stderr missing confirmation guidance:\n%s", result.Stderr)
	}
	server.AssertNoRequests(t)
}

func TestRawAPINonBillablePlanGatedDeleteDoesNotRequireAcknowledgement(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/files/f_123":
			if r.Method != http.MethodDelete {
				t.Fatalf("request = %s %s", r.Method, r.URL.Path)
			}
			fmt.Fprint(w, `{"data":{"id":"f_123","deleted":true},"meta":{"request_id":"req-delete"}}`)
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"api", "delete", "/files/f_123",
		"--no-prompt",
		"--json",
	)...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("raw delete result = %#v", result)
	}
	server.AssertNoIdempotencyKey(t, 0)
	server.AssertIdempotencyKeyMatches(t, 1, chabIdempotencyPattern())
	if !strings.Contains(result.Stdout, `"operation_key": "files.delete"`) || !strings.Contains(result.Stdout, `"deleted": true`) {
		t.Fatalf("stdout missing raw action wrapper:\n%s", result.Stdout)
	}
}

func TestRawAPIUnknownOutcomeWritesActionRecoveryObject(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/examples/echo":
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response writer does not support hijacking")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatalf("Hijack() error = %v", err)
			}
			_ = conn.Close()
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"api", "post", "/examples/echo",
		"--body", `{"event":"demo"}`,
		"--idempotency-key", "idem-unknown-123",
		"--json",
	)...)
	if result.ExitCode == cli.ExitSuccess || result.Err == nil || result.Stdout == "" {
		t.Fatalf("raw unknown result = %#v", result)
	}
	if !strings.Contains(result.Stdout, `"state": "unknown"`) || !strings.Contains(result.Stdout, `"resume_hint": "chab operations resume`) {
		t.Fatalf("stdout missing recovery action object:\n%s", result.Stdout)
	}
	if !strings.Contains(result.Stderr, "api_network_error") {
		t.Fatalf("stderr missing network error:\n%s", result.Stderr)
	}
}

func TestRawAPIMarkedDenialCannotReplay(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
	}{
		{"rate_limited_429", http.StatusTooManyRequests, "rate_limited"},
		{"provider_unavailable_503", http.StatusServiceUnavailable, "provider_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, _ := configuredChabAuthState(t)
			starts := 0
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/me":
					requireWhoamiRequest(t, r)
					fmt.Fprint(w, whoamiEnvelope())
				case "/v1/examples/echo":
					starts++
					w.Header().Set("X-Chab-Idempotency-Outcome", "released")
					w.Header().Set("Retry-After", "1")
					testutil.WriteJSON(t, w, tc.status, testutil.ErrorEnvelope(tc.code, "denied after claim", "req-raw-denied", nil))
				default:
					t.Fatalf("request = %s %s", r.Method, r.URL.Path)
				}
			})
			args := state.APIArgs(server, "api", "post", "/examples/echo", "--body", `{"event":"demo"}`, "--idempotency-key", "idem-raw-denied", "--json")
			first := testutil.RunCommandWith(t, chabAuthOptions(), args...)
			if first.ExitCode == cli.ExitSuccess || starts != 1 || !strings.Contains(first.Stdout, `"state": "denied"`) || !strings.Contains(first.Stdout, `"can_resume": false`) {
				t.Fatalf("first marked denial = %#v, starts=%d", first, starts)
			}
			second := testutil.RunCommandWith(t, chabAuthOptions(), args...)
			if second.ExitCode == cli.ExitSuccess || starts != 1 {
				t.Fatalf("terminal raw action replayed: %#v, starts=%d", second, starts)
			}
		})
	}
}

func TestRawAPIUnknownReplaySettledErrorStaysUnknown(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	starts := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/examples/echo":
			starts++
			if starts == 1 {
				testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("server_error", "uncertain", "req-first", nil))
			} else {
				w.Header().Set("X-Chab-Idempotency-Outcome", "settled")
				testutil.WriteJSON(t, w, http.StatusServiceUnavailable, testutil.ErrorEnvelope("provider_unavailable", "saved error", "req-replay", nil))
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	args := state.APIArgs(server, "api", "post", "/examples/echo", "--body", `{"event":"demo"}`, "--idempotency-key", "idem-raw-unknown-replay", "--json")
	first := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	second := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	if first.ExitCode == cli.ExitSuccess || second.ExitCode == cli.ExitSuccess || starts != 2 || !strings.Contains(second.Stdout, `"state": "unknown"`) || !strings.Contains(second.Stdout, `"can_resume": true`) {
		t.Fatalf("raw unknown replay = first %#v, second %#v, starts=%d", first, second, starts)
	}
}

func TestRawAPIMalformedMarkedDenialCannotReplay(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	starts := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/examples/echo":
			starts++
			w.Header().Set("X-Chab-Idempotency-Outcome", "released")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":`)
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	})
	args := state.APIArgs(server, "api", "post", "/examples/echo", "--body", `{"event":"demo"}`, "--idempotency-key", "idem-raw-malformed-denied", "--json")
	first := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	if first.ExitCode == cli.ExitSuccess || starts != 1 || !strings.Contains(first.Stdout, `"state": "denied"`) {
		t.Fatalf("malformed marked denial = %#v, starts=%d", first, starts)
	}
	second := testutil.RunCommandWith(t, chabAuthOptions(), args...)
	if second.ExitCode == cli.ExitSuccess || starts != 1 {
		t.Fatalf("malformed marked denial replayed: %#v, starts=%d", second, starts)
	}
}

func TestRawAPISimulatedAsyncEchoIsRecoverable(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/examples/echo":
			fmt.Fprint(w, `{"operation":{"id":"op_raw_echo_async_123","status":"queued","operation_key":"examples.echo","family":"examples","result_available":false},"meta":{"request_id":"req-raw-echo-async"}}`)
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"api", "post", "/examples/echo",
		"--body", `{"simulate_async":true}`,
		"--idempotency-key", "idem-raw-echo-async",
		"--json",
	)...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("raw async echo result = %#v", result)
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
		t.Fatalf("raw async echo JSON error = %v: %s", err, result.Stdout)
	}
	if out.Action.State != "accepted" || out.Action.LastStatus != "queued" ||
		out.OperationID != "op_raw_echo_async_123" ||
		out.LocalRecovery.State != "accepted" ||
		!out.LocalRecovery.CanResume ||
		out.LocalRecovery.ResumeHint != "chab operations wait op_raw_echo_async_123" {
		t.Fatalf("raw async echo output = %#v", out)
	}
}

func TestRawAPIBulkCancelSynchronousSummaryCompletes(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/operations/cancel":
			if r.Method != http.MethodPost {
				t.Fatalf("request = %s %s", r.Method, r.URL.Path)
			}
			fmt.Fprint(w, `{"data":{"cancelled":[{"id":"op_cancelled","status":"cancelling","operation_key":"examples.echo","family":"examples","result_available":false}],"already_terminal":[],"unsupported":[],"not_found_or_not_authorized":0,"counts":{"cancelled":1,"already_terminal":0,"unsupported":0,"not_found_or_not_authorized":0},"capped":false},"meta":{"request_id":"req-raw-bulk-cancel"}}`)
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"api", "post", "/operations/cancel",
		"--body", `{"status":"running"}`,
		"--yes",
		"--json",
	)...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("raw bulk cancel result = %#v", result)
	}
	var out struct {
		Action struct {
			State string `json:"state"`
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
		t.Fatalf("raw bulk cancel JSON error = %v: %s", err, result.Stdout)
	}
	if out.Action.State != "completed" || out.OperationID != "" ||
		out.LocalRecovery.State != "completed" || out.LocalRecovery.CanResume ||
		out.Server.Counts.Cancelled != 1 {
		t.Fatalf("raw bulk cancel output = %#v", out)
	}
}

func TestRawAPIIncludeMetaWrapsCataloguedActionOutput(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/examples/echo":
			fmt.Fprint(w, `{"data":{"echo":{"ok":true}},"meta":{"request_id":"req-raw-meta"}}`)
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"api", "post", "/examples/echo",
		"--body", `{"data":{"ok":true}}`,
		"--idempotency-key", "idem-raw-meta",
		"--include-meta",
		"--jq", ".data.server.echo.ok",
	)...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("raw include-meta result = %#v", result)
	}
	if strings.TrimSpace(result.Stdout) != "true" {
		t.Fatalf("raw include-meta jq stdout = %q, want true", result.Stdout)
	}

	meta := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"api", "post", "/examples/echo",
		"--body", `{"data":{"ok":true}}`,
		"--idempotency-key", "idem-raw-meta-2",
		"--include-meta",
		"--jq", ".meta.request_id",
	)...)
	if meta.ExitCode != cli.ExitSuccess || meta.Err != nil || meta.Stderr != "" {
		t.Fatalf("raw include-meta request id result = %#v", meta)
	}
	if strings.TrimSpace(meta.Stdout) != `"req-raw-meta"` {
		t.Fatalf("raw include-meta request id stdout = %q", meta.Stdout)
	}
}

func TestRawAPIPersistenceFailureRetainsAcceptedOperationID(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	actionDir := filepath.Join(filepath.Dir(state.ConfigPath), "actions")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/examples/echo":
			replaceActionRecordWithDirectory(t, actionDir)
			fmt.Fprint(w, `{"operation":{"id":"op_accepted_123","status":"queued","operation_key":"examples.echo","family":"examples","result_available":false},"meta":{"request_id":"req-accepted"}}`)
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"api", "post", "/examples/echo",
		"--body", `{"simulate_async":true}`,
		"--idempotency-key", "idem-raw-persist-failure",
		"--plain",
	)...)
	if result.ExitCode == cli.ExitSuccess || result.Err == nil {
		t.Fatalf("raw persistence failure result = %#v", result)
	}
	if strings.TrimSpace(result.Stdout) != "op_accepted_123" {
		t.Fatalf("raw persistence failure plain stdout = %q", result.Stdout)
	}
	if !strings.Contains(result.Stderr, "failed to persist local recovery state") {
		t.Fatalf("stderr missing persistence failure:\n%s", result.Stderr)
	}
}

func TestRawAPIServerDryRunSkipsJournalIdempotencyAndConfirmation(t *testing.T) {
	state, key := configuredChabAuthState(t)
	actionDir := filepath.Join(filepath.Dir(state.ConfigPath), "actions")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/screenshots/url" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"data":{"valid":true,"dry_run":true},"meta":{"request_id":"req-raw-dry"}}`)
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server,
		"api", "post", "/screenshots/url",
		"--body", `{"url":"https://example.test","dry_run":true}`,
		"--no-prompt",
		"--json",
	)...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("raw server dry-run result = %#v", result)
	}
	server.AssertBearer(t, key)
	server.AssertNoIdempotencyKey(t, 0)
	if got, want := server.BodyString(t, 0), `{"url":"https://example.test","dry_run":true}`; got != want {
		t.Fatalf("request body = %s, want exact raw bytes %s", got, want)
	}
	assertNotExists(t, actionDir)
	if !strings.Contains(result.Stdout, `"dry_run": true`) || !strings.Contains(result.Stdout, `"valid": true`) {
		t.Fatalf("stdout missing server dry-run result:\n%s", result.Stdout)
	}
}

func assertRawActionRecord(t *testing.T, state testutil.State, idem, key string) {
	t.Helper()
	actionDir := filepath.Join(filepath.Dir(state.ConfigPath), "actions")
	entries, err := os.ReadDir(actionDir)
	if err != nil {
		t.Fatalf("read action dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("action files = %d, want 1", len(entries))
	}
	raw, err := os.ReadFile(filepath.Join(actionDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read action record: %v", err)
	}
	var record struct {
		OperationKey   string `json:"operation_key"`
		EncoderVersion string `json:"encoder_version"`
		IdempotencyKey string `json:"idempotency_key"`
		State          string `json:"state"`
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("decode action record: %v", err)
	}
	if record.OperationKey != "examples.echo" || record.EncoderVersion != "raw-json-v1" || record.IdempotencyKey != idem || record.State != "completed" {
		t.Fatalf("action record = %#v", record)
	}
	for _, forbidden := range []string{`"pin"`, `"event"`, "123456", key} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("action record leaked forbidden value %q: %s", forbidden, string(raw))
		}
	}
}
