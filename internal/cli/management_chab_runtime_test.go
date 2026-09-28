package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

const managementApprovalHeader = "X-Chab-Management-Approval"

func TestManagementProjectCRUDUsesCursorWrappersAndNulls(t *testing.T) {
	state, apiKey := configuredChabCreditsState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/projects":
			if unexpected := firstPresentQuery(r, "status", "search", "sort", "page", "per_page"); unexpected != "" {
				t.Fatalf("project list sent unsupported query %q", unexpected)
			}
			if got := r.URL.Query().Get("limit"); got != "1" {
				t.Fatalf("project list limit = %q, want 1", got)
			}
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data": []map[string]any{projectFixture(42, nil, nil)},
				"meta": map[string]any{
					"limit":       1,
					"has_more":    false,
					"next_cursor": nil,
					"prev_cursor": nil,
				},
				"request_id": "req-project-list",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects":
			body := decodeObjectBody(t, r)
			assertBodyField(t, body, "name", "Demo")
			assertBodyNull(t, body, "description")
			assertBodyNull(t, body, "url")
			testutil.WriteJSON(t, w, http.StatusCreated, map[string]any{
				"data":       map[string]any{"project": projectFixture(42, nil, nil)},
				"request_id": "req-project-create",
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/projects/42":
			body := decodeObjectBody(t, r)
			assertBodyNull(t, body, "description")
			assertBodyNull(t, body, "url")
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data":       map[string]any{"project": projectFixture(42, nil, nil)},
				"request_id": "req-project-update",
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/projects/42":
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data":       map[string]any{"project": map[string]any{"id": 42, "deleted": true}},
				"request_id": "req-project-delete",
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	list := runManagement(t, state, server, "project", "list", "--limit", "1", "--json", "--include-meta")
	assertCommandSuccess(t, list)
	var listOut struct {
		Data []struct {
			ID          string  `json:"id"`
			Description *string `json:"description"`
			URL         *string `json:"url"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"request_id"`
		} `json:"meta"`
	}
	mustUnmarshal(t, list.Stdout, &listOut)
	if len(listOut.Data) != 1 || listOut.Data[0].ID != "42" || listOut.Data[0].Description != nil || listOut.Data[0].URL != nil || listOut.Meta.RequestID != "req-project-list" {
		t.Fatalf("project list output = %#v", listOut)
	}

	create := runManagement(t, state, server, "project", "create", "--name", "Demo", "--description-null", "--url-null", "--idempotency-key", "idem-project-create", "--json")
	assertCommandSuccess(t, create)
	var createOut struct {
		ID          string  `json:"id"`
		Description *string `json:"description"`
		URL         *string `json:"url"`
	}
	mustUnmarshal(t, create.Stdout, &createOut)
	if createOut.ID != "42" || createOut.Description != nil || createOut.URL != nil {
		t.Fatalf("project create output = %#v", createOut)
	}

	update := runManagement(t, state, server, "project", "update", "42", "--description-null", "--url-null", "--idempotency-key", "idem-project-update", "--json")
	assertCommandSuccess(t, update)

	del := runManagement(t, state, server, "project", "delete", "42", "--idempotency-key", "idem-project-delete", "--yes", "--json")
	assertCommandSuccess(t, del)
	var deleteOut struct {
		ID      string `json:"id"`
		Deleted bool   `json:"deleted"`
	}
	mustUnmarshal(t, del.Stdout, &deleteOut)
	if deleteOut.ID != "42" || !deleteOut.Deleted {
		t.Fatalf("project delete output = %#v", deleteOut)
	}

	if server.Count() != 4 {
		t.Fatalf("request count = %d, want 4", server.Count())
	}
	server.AssertBearer(t, apiKey)
	server.AssertNoIdempotencyKey(t, 0)
	server.AssertIdempotencyKey(t, 1, "idem-project-create")
	server.AssertIdempotencyKey(t, 2, "idem-project-update")
	server.AssertIdempotencyKey(t, 3, "idem-project-delete")

	rejected := testutil.RunCommandWith(t, managementOptions(), state.APIArgs(server, "project", "list", "--status", "active")...)
	if rejected.ExitCode != cli.ExitUsage || !strings.Contains(rejected.Stderr, "unknown flag: --status") {
		t.Fatalf("project list unsupported flag result = %#v", rejected)
	}
	if server.Count() != 4 {
		t.Fatalf("unsupported project list flag contacted API; request count = %d", server.Count())
	}
}

func TestProjectCommandsRejectMissingProjectWrapper(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		method     string
		path       string
		statusCode int
	}{
		{
			name:       "show",
			args:       []string{"project", "show", "42", "--json"},
			method:     http.MethodGet,
			path:       "/v1/projects/42",
			statusCode: http.StatusOK,
		},
		{
			name:       "create",
			args:       []string{"project", "create", "--name", "Demo", "--idempotency-key", "idem-project-create-missing", "--json"},
			method:     http.MethodPost,
			path:       "/v1/projects",
			statusCode: http.StatusCreated,
		},
		{
			name:       "delete",
			args:       []string{"project", "delete", "42", "--idempotency-key", "idem-project-delete-missing", "--yes", "--json"},
			method:     http.MethodDelete,
			path:       "/v1/projects/42",
			statusCode: http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, _ := configuredChabCreditsState(t)
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != tt.path {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				testutil.WriteJSON(t, w, tt.statusCode, map[string]any{
					"data":       map[string]any{},
					"request_id": "req-project-missing-wrapper",
				})
			})

			result := runManagement(t, state, server, tt.args...)
			if result.ExitCode == cli.ExitSuccess || !strings.Contains(result.Stderr, "data.project") {
				t.Fatalf("missing project wrapper result = %#v", result)
			}
			if server.Count() != 1 {
				t.Fatalf("request count = %d, want 1", server.Count())
			}
		})
	}
}

func TestProjectShowRedactsRegisteredCredentialInSuccessfulOutput(t *testing.T) {
	state, apiKey := configuredChabCreditsState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/projects/42" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		project := projectFixture(42, apiKey, nil)
		project["name"] = apiKey
		testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
			"data":       map[string]any{"project": project},
			"request_id": "req-project-show-redact",
		})
	})

	result := runManagement(t, state, server, "project", "show", "42", "--json")
	assertCommandSuccess(t, result)
	assertNotContains(t, result.Stdout+result.Stderr, apiKey)
	var out struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	mustUnmarshal(t, result.Stdout, &out)
	if out.Name != "[REDACTED]" || out.Description == nil || *out.Description != "[REDACTED]" {
		t.Fatalf("project show redaction = %#v", out)
	}
	server.AssertBearer(t, apiKey)
}

func TestProjectListRedactsRegisteredCredentialInSuccessfulOutput(t *testing.T) {
	state, apiKey := configuredChabCreditsState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/projects" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		project := projectFixture(42, apiKey, nil)
		project["name"] = apiKey
		testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
			"data": []map[string]any{project},
			"meta": map[string]any{
				"limit":       1,
				"has_more":    false,
				"next_cursor": nil,
				"prev_cursor": nil,
			},
			"request_id": "req-project-list-redact",
		})
	})

	result := runManagement(t, state, server, "project", "list", "--json")
	assertCommandSuccess(t, result)
	assertNotContains(t, result.Stdout+result.Stderr, apiKey)
	var out []struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	mustUnmarshal(t, result.Stdout, &out)
	if len(out) != 1 || out[0].Name != "[REDACTED]" || out[0].Description == nil || *out[0].Description != "[REDACTED]" {
		t.Fatalf("project list redaction = %#v", out)
	}
	server.AssertBearer(t, apiKey)
}

func TestTokenCreateWritesPrivateSecretAndDoesNotRetrySecretRoute(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	proof := "approval-proof-token-create"
	proofFile := writeProofFixture(t, state, proof)
	secretOut := filepath.Join(state.Dir, "child-token.json")
	if privateOutputUnsupported() {
		server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("private-output refusal contacted API: %s %s", r.Method, r.URL.Path)
		})
		assertPrivateOutputRefusedBeforeHTTP(t, state, server, secretOut,
			"tokens", "create",
			"--name", "worker",
			"--permission", "api:projects:read",
			"--spending-mode", "none",
			"--project-mode", "none",
			"--approval-proof-file", proofFile,
			"--secret-out", secretOut,
			"--json",
		)
		return
	}
	tokenSecret := testutil.FakeKey("child_token_secret")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/tokens" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get(managementApprovalHeader) != proof {
			t.Fatal("token create approval proof header mismatch")
		}
		body := decodeObjectBody(t, r)
		assertBodyField(t, body, "name", "worker")
		testutil.WriteJSON(t, w, http.StatusCreated, map[string]any{
			"data": map[string]any{
				"token": map[string]any{
					"token_public_id": "ak_child_public",
					"name":            "worker",
					"permissions":     []string{"api:projects:read"},
					"policy_revision": 1,
					"disabled":        false,
					"revoked":         false,
					"expires_at":      nil,
					"last_used_at":    nil,
				},
				"access_token": tokenSecret,
			},
			"request_id": "req-token-create",
		})
	})

	result := runManagement(t, state, server,
		"tokens", "create",
		"--name", "worker",
		"--permission", "api:projects:read",
		"--spending-mode", "none",
		"--project-mode", "none",
		"--approval-proof-file", proofFile,
		"--secret-out", secretOut,
		"--json",
	)
	assertCommandSuccess(t, result)
	assertNotContains(t, result.Stdout+result.Stderr, tokenSecret, proof)
	assertPrivateJSONFile(t, secretOut, func(raw string) {
		if !strings.Contains(raw, tokenSecret) || !strings.Contains(raw, `"operation": "tokens.create"`) {
			t.Fatalf("token secret file missing expected private fields")
		}
	})
	server.AssertIdempotencyKeyMatches(t, 0, chabIdempotencyPattern())

	failState, _ := configuredChabCreditsState(t)
	failProof := "approval-proof-no-retry"
	failProofFile := writeProofFixture(t, failState, failProof)
	failServer := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/tokens" {
			t.Fatalf("unexpected retry test request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get(managementApprovalHeader) != failProof {
			t.Fatal("retry test approval proof header mismatch")
		}
		writeAPIError(w, http.StatusInternalServerError, "server_error", "transient secret-route failure")
	})
	failed := runManagement(t, failState, failServer,
		"tokens", "create",
		"--name", "worker",
		"--permission", "api:projects:read",
		"--approval-proof-file", failProofFile,
		"--secret-out", filepath.Join(failState.Dir, "failed-child-token.json"),
		"--json",
	)
	if failed.ExitCode != cli.ExitAPI || failed.Err == nil {
		t.Fatalf("token create failure result = %#v", failed)
	}
	if failServer.Count() != 1 {
		t.Fatalf("secret-producing token route was retried; request count = %d", failServer.Count())
	}
}

func TestTokenCreateRemovesReservationWhenAuthFailsBeforeRequest(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	proofFile := writeProofFixture(t, state, "approval-proof-auth-missing")
	secretOut := filepath.Join(state.Dir, "missing-auth-token.json")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("pre-request credential failure contacted API: %s %s", r.Method, r.URL.Path)
	})
	if privateOutputUnsupported() {
		assertPrivateOutputRefusedBeforeHTTP(t, state, server, secretOut,
			"tokens", "create",
			"--name", "worker",
			"--permission", "api:projects:read",
			"--approval-proof-file", proofFile,
			"--secret-out", secretOut,
			"--json",
		)
		return
	}
	if err := os.Remove(state.AuthPath); err != nil {
		t.Fatalf("remove auth fixture: %v", err)
	}

	result := runManagement(t, state, server,
		"tokens", "create",
		"--name", "worker",
		"--permission", "api:projects:read",
		"--approval-proof-file", proofFile,
		"--secret-out", secretOut,
		"--json",
	)
	if result.ExitCode == cli.ExitSuccess || !strings.Contains(result.Stderr, "CHAB_API_KEY") {
		t.Fatalf("token create missing-auth result = %#v", result)
	}
	assertNotExists(t, secretOut)
	if server.Count() != 0 {
		t.Fatalf("pre-request credential failure contacted API; request count = %d", server.Count())
	}
}

func TestApprovalWaitWritesProofWithStoredContext(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	proofOut := filepath.Join(state.Dir, "approval-proof.json")
	if privateOutputUnsupported() {
		server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("private-output refusal contacted API: %s %s", r.Method, r.URL.Path)
		})
		assertPrivateOutputRefusedBeforeHTTP(t, state, server, proofOut,
			"tokens", "approvals", "wait", "map_test_approval",
			"--proof-out", proofOut,
			"--json",
		)
		return
	}
	server := testutil.NewAPIServer(t, testutil.SequenceHandler(t,
		func(w http.ResponseWriter, r *http.Request) {
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/v1/management-approvals" {
				t.Fatalf("unexpected approval create request %s %s", r.Method, r.URL.Path)
			}
			body := decodeObjectBody(t, r)
			assertBodyField(t, body, "action", "tokens.create")
			assertBodyField(t, body, "target_type", "team")
			assertBodyField(t, body, "target_id", "42")
			testutil.WriteJSON(t, w, http.StatusCreated, map[string]any{
				"data": map[string]any{
					"approval_id":      "map_test_approval",
					"verification_uri": "/teams/42/api-tokens/management-approvals/map_test_approval",
					"expires_at":       "2026-08-16T10:05:00+00:00",
					"next_check_at":    "2026-08-16T10:00:02+00:00",
				},
				"request_id": "req-approval-create",
			})
		},
		func(w http.ResponseWriter, r *http.Request) {
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v1/management-approvals/map_test_approval" {
				t.Fatalf("unexpected approval wait request %s %s", r.Method, r.URL.Path)
			}
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"approval_id":   "map_test_approval",
					"state":         "approved",
					"expires_at":    "2026-08-16T10:05:00+00:00",
					"next_check_at": nil,
					"proof":         "approval-proof-returned-once",
				},
				"request_id": "req-approval-proof",
			})
		},
	))

	create := runManagement(t, state, server,
		"tokens", "approvals", "create",
		"--action", "tokens.create",
		"--mutation", `{"name":"worker","permissions":["api:projects:read"]}`,
		"--idempotency-key", "idem-approval-create",
		"--json",
	)
	assertCommandSuccess(t, create)
	wait := runManagement(t, state, server, "tokens", "approvals", "wait", "map_test_approval", "--proof-out", proofOut, "--json")
	assertCommandSuccess(t, wait)
	assertNotContains(t, wait.Stdout+wait.Stderr, "approval-proof-returned-once")
	assertPrivateJSONFile(t, proofOut, func(raw string) {
		for _, want := range []string{
			`"approval_id": "map_test_approval"`,
			`"action": "tokens.create"`,
			`"target_type": "team"`,
			`"target_id": "42"`,
			`"issuing_token_public_id": "ak_01HY0000000000000000000000"`,
			`"proof": "approval-proof-returned-once"`,
		} {
			if !strings.Contains(raw, want) {
				t.Fatalf("proof file missing %q", want)
			}
		}
	})
	server.AssertIdempotencyKey(t, 1, "idem-approval-create")
	server.AssertNoIdempotencyKey(t, 3)
}

func TestBillingPurchaseCreateReportsUnknownActionRecovery(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	proofFile := writeProofFixture(t, state, "approval-proof-purchase-unknown")
	server := testutil.NewAPIServer(t, testutil.SequenceHandler(t,
		func(w http.ResponseWriter, r *http.Request) {
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/v1/billing/purchases" {
				t.Fatalf("unexpected purchase create request %s %s", r.Method, r.URL.Path)
			}
			writeAPIError(w, http.StatusInternalServerError, "server_error", "ambiguous purchase outcome")
		},
	))

	result := runManagement(t, state, server,
		"billing", "purchases", "create",
		"--package-id", "credits_1000",
		"--approval-proof-file", proofFile,
		"--yes",
	)
	if result.ExitCode != cli.ExitAPI || result.Err == nil ||
		!strings.Contains(result.Stderr, "purchase outcome is unknown") ||
		!strings.Contains(result.Stderr, "Local action") ||
		!strings.Contains(result.Stderr, "chab operations resume") ||
		!strings.Contains(result.Stderr, "generated idempotency key") {
		t.Fatalf("purchase unknown recovery result = %#v", result)
	}
	if server.Count() != 2 {
		t.Fatalf("request count = %d, want 2", server.Count())
	}
}

func TestBillingPurchaseCreateJSONDenialIsNotRecoverable(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	proofFile := writeProofFixture(t, state, "approval-proof-purchase-json")
	server := testutil.NewAPIServer(t, testutil.SequenceHandler(t,
		func(w http.ResponseWriter, r *http.Request) {
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/v1/billing/purchases" {
				t.Fatalf("unexpected purchase create request %s %s", r.Method, r.URL.Path)
			}
			writeAPIError(w, http.StatusForbidden, "api_scope_missing", "missing billing scope")
		},
	))

	result := runManagement(t, state, server,
		"billing", "purchases", "create",
		"--package-id", "credits_1000",
		"--approval-proof-file", proofFile,
		"--yes",
		"--json",
	)
	if result.ExitCode != cli.ExitForbidden || result.Err == nil || result.Stdout != "" {
		t.Fatalf("purchase JSON error result = %#v", result)
	}
	var envelope struct {
		Error struct {
			Code     string `json:"code"`
			Message  string `json:"message"`
			Recovery *struct {
				CanResume bool `json:"can_resume"`
			} `json:"recovery"`
		} `json:"error"`
	}
	mustUnmarshal(t, result.Stderr, &envelope)
	if envelope.Error.Code != "api_scope_missing" ||
		envelope.Error.Message != "missing billing scope" ||
		(envelope.Error.Recovery != nil && envelope.Error.Recovery.CanResume) {
		t.Fatalf("purchase JSON error envelope = %#v", envelope)
	}
	actionID := singleActionID(t, state)
	record, err := os.ReadFile(filepath.Join(filepath.Dir(state.ConfigPath), "actions", actionID+".json"))
	if err != nil || !strings.Contains(string(record), `"state": "denied"`) {
		t.Fatalf("purchase denial journal = %q, error=%v", record, err)
	}
	if server.Count() != 2 {
		t.Fatalf("request count = %d, want 2", server.Count())
	}
}

func TestBillingPurchaseMarkedDenialCannotReplay(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	proofFile := writeProofFixture(t, state, "approval-proof-purchase-marked")
	starts := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/billing/purchases":
			starts++
			w.Header().Set("X-Chab-Idempotency-Outcome", "released")
			w.Header().Set("Retry-After", "1")
			testutil.WriteJSON(t, w, http.StatusTooManyRequests, testutil.ErrorEnvelope("rate_limited", "denied after claim", "req-purchase-denied", nil))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	args := []string{"billing", "purchases", "create", "--package-id", "credits_1000", "--approval-proof-file", proofFile, "--idempotency-key", "idem-purchase-denied", "--yes", "--json"}
	first := runManagement(t, state, server, args...)
	if first.ExitCode == cli.ExitSuccess || starts != 1 || strings.Contains(first.Stderr, `"can_resume": true`) {
		t.Fatalf("first marked purchase denial = %#v, starts=%d", first, starts)
	}
	actionID := singleActionID(t, state)
	record, err := os.ReadFile(filepath.Join(filepath.Dir(state.ConfigPath), "actions", actionID+".json"))
	if err != nil || !strings.Contains(string(record), `"state": "denied"`) {
		t.Fatalf("purchase denial journal = %q, error=%v", record, err)
	}
	second := runManagement(t, state, server, args...)
	if second.ExitCode == cli.ExitSuccess || starts != 1 {
		t.Fatalf("denied purchase replayed: %#v, starts=%d", second, starts)
	}
}

func TestBillingPurchaseUnknownReplaySettledErrorStaysUnknown(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	proofFile := writeProofFixture(t, state, "approval-proof-purchase-replay-unknown")
	starts := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case "/v1/billing/purchases":
			starts++
			if starts == 1 {
				testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("server_error", "uncertain", "req-first", nil))
			} else {
				w.Header().Set("X-Chab-Idempotency-Outcome", "settled")
				testutil.WriteJSON(t, w, http.StatusConflict, testutil.ErrorEnvelope("idempotency_key_conflict", "saved response", "req-replay", nil))
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	args := []string{"billing", "purchases", "create", "--package-id", "credits_1000", "--approval-proof-file", proofFile, "--idempotency-key", "idem-purchase-unknown-replay", "--yes", "--json"}
	first := runManagement(t, state, server, args...)
	second := runManagement(t, state, server, args...)
	actionID := singleActionID(t, state)
	record, err := os.ReadFile(filepath.Join(filepath.Dir(state.ConfigPath), "actions", actionID+".json"))
	if first.ExitCode == cli.ExitSuccess || second.ExitCode == cli.ExitSuccess || starts != 2 || err != nil || !strings.Contains(string(record), `"state": "unknown"`) {
		t.Fatalf("purchase unknown replay = first %#v, second %#v, starts=%d, journal=%q, read=%v", first, second, starts, record, err)
	}
}

func TestOperationResumePurchaseActionUsesPurchaseLifecycle(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	proofFile := writeProofFixture(t, state, "approval-proof-purchase-resume")
	server := testutil.NewAPIServer(t, testutil.SequenceHandler(t,
		func(w http.ResponseWriter, r *http.Request) {
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/v1/billing/purchases" {
				t.Fatalf("unexpected interrupted purchase request %s %s", r.Method, r.URL.Path)
			}
			body := decodeObjectBody(t, r)
			assertBodyField(t, body, "package_id", "credits_1000")
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `{"data":{"id":"pur_review","package_id":"credits_1000","state":"pending"`)
		},
		func(w http.ResponseWriter, r *http.Request) {
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/v1/billing/purchases" {
				t.Fatalf("unexpected resumed purchase request %s %s", r.Method, r.URL.Path)
			}
			body := decodeObjectBody(t, r)
			assertBodyField(t, body, "package_id", "credits_1000")
			testutil.WriteJSON(t, w, http.StatusAccepted, map[string]any{
				"data": map[string]any{
					"id":            "pur_review",
					"package_id":    "credits_1000",
					"state":         "pending",
					"next_check_at": "2026-08-16T10:00:05+00:00",
					"created_at":    "2026-08-16T10:00:00+00:00",
				},
				"request_id": "req-purchase-resume",
			})
		},
		func(w http.ResponseWriter, r *http.Request) {
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v1/billing/purchases/pur_review" {
				t.Fatalf("unexpected purchase follow request %s %s", r.Method, r.URL.Path)
			}
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"id":            "pur_review",
					"package_id":    "credits_1000",
					"state":         "fulfilled",
					"next_check_at": nil,
					"created_at":    "2026-08-16T10:00:00+00:00",
				},
				"request_id": "req-purchase-follow",
			})
		},
	))

	create := runManagement(t, state, server,
		"billing", "purchases", "create",
		"--package-id", "credits_1000",
		"--approval-proof-file", proofFile,
		"--idempotency-key", "idem-purchase-resume",
		"--yes",
		"--json",
	)
	if create.ExitCode != cli.ExitAPI || create.Err == nil {
		t.Fatalf("interrupted purchase create result = %#v", create)
	}
	actionID := singleActionID(t, state)
	firstResume := runManagement(t, state, server,
		"operations", "resume", actionID,
		"--input", `{"package_id":"credits_1000"}`,
		"--json",
	)
	assertCommandSuccess(t, firstResume)
	var firstOut struct {
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
		LocalPersistence struct {
			State string `json:"state"`
		} `json:"local_persistence"`
		Server struct {
			State string `json:"state"`
		} `json:"server"`
	}
	mustUnmarshal(t, firstResume.Stdout, &firstOut)
	if firstOut.Action.State != "accepted" ||
		firstOut.Action.LastStatus != "pending" ||
		firstOut.OperationID != "pur_review" ||
		firstOut.LocalRecovery.State != "accepted" ||
		!firstOut.LocalRecovery.CanResume ||
		firstOut.LocalRecovery.ResumeHint != "chab billing purchases wait pur_review" ||
		firstOut.LocalPersistence.State != "persisted" ||
		firstOut.Server.State != "pending" {
		t.Fatalf("first purchase resume output = %#v", firstOut)
	}

	secondResume := runManagement(t, state, server,
		"operations", "resume", actionID,
		"--json",
	)
	assertCommandSuccess(t, secondResume)
	var secondOut struct {
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
		Server struct {
			State string `json:"state"`
		} `json:"server"`
	}
	mustUnmarshal(t, secondResume.Stdout, &secondOut)
	if secondOut.Action.State != "completed" ||
		secondOut.Action.LastStatus != "fulfilled" ||
		secondOut.OperationID != "pur_review" ||
		secondOut.LocalRecovery.State != "completed" ||
		secondOut.LocalRecovery.CanResume ||
		secondOut.LocalRecovery.ResumeHint != "" ||
		secondOut.Server.State != "fulfilled" {
		t.Fatalf("second purchase resume output = %#v", secondOut)
	}
	server.AssertIdempotencyKey(t, 1, "idem-purchase-resume")
	server.AssertIdempotencyKey(t, 3, "idem-purchase-resume")
	server.AssertNoIdempotencyKey(t, 5)
	if server.Count() != 6 {
		t.Fatalf("request count = %d, want 6", server.Count())
	}
}

func TestBillingPurchaseCreateReusesAcceptedAction(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	proof := "approval-proof-purchase"
	proofFile := writeProofFixture(t, state, proof)
	server := testutil.NewAPIServer(t, testutil.SequenceHandler(t,
		func(w http.ResponseWriter, r *http.Request) {
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/v1/billing/purchases" {
				t.Fatalf("unexpected purchase create request %s %s", r.Method, r.URL.Path)
			}
			if r.Header.Get(managementApprovalHeader) != proof {
				t.Fatal("purchase approval proof header mismatch")
			}
			body := decodeObjectBody(t, r)
			assertBodyField(t, body, "package_id", "credits_1000")
			testutil.WriteJSON(t, w, http.StatusAccepted, map[string]any{
				"data": map[string]any{
					"id":            "pur_test_purchase",
					"package_id":    "credits_1000",
					"state":         "pending",
					"next_check_at": "2026-08-16T10:00:05+00:00",
					"created_at":    "2026-08-16T10:00:00+00:00",
				},
				"request_id": "req-purchase-create",
			})
		},
		func(w http.ResponseWriter, r *http.Request) {
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v1/billing/purchases/pur_test_purchase" {
				t.Fatalf("unexpected purchase follow request %s %s", r.Method, r.URL.Path)
			}
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"id":            "pur_test_purchase",
					"package_id":    "credits_1000",
					"state":         "fulfilled",
					"next_check_at": nil,
					"created_at":    "2026-08-16T10:00:00+00:00",
				},
				"request_id": "req-purchase-follow",
			})
		},
	))

	first := runManagement(t, state, server,
		"billing", "purchases", "create",
		"--package-id", "credits_1000",
		"--approval-proof-file", proofFile,
		"--idempotency-key", "idem-purchase-create",
		"--yes",
		"--json",
	)
	assertCommandSuccess(t, first)
	second := runManagement(t, state, server,
		"billing", "purchases", "create",
		"--package-id", "credits_1000",
		"--approval-proof-file", proofFile,
		"--idempotency-key", "idem-purchase-create",
		"--yes",
		"--json",
	)
	assertCommandSuccess(t, second)
	if server.Count() != 4 {
		t.Fatalf("request count = %d, want 4", server.Count())
	}
	server.AssertIdempotencyKey(t, 1, "idem-purchase-create")
	server.AssertNoIdempotencyKey(t, 3)
	assertPurchaseActionRecord(t, state, "idem-purchase-create")
	if !strings.Contains(second.Stdout, `"state": "fulfilled"`) {
		t.Fatalf("second purchase output did not follow accepted resource:\n%s", second.Stdout)
	}
}

func TestBillingPurchaseCreateRefusesExpiredUnknownAction(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	proofFile := writeProofFixture(t, state, "approval-proof-purchase-expired")
	var purchaseRequests int
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		case r.Method == http.MethodPost && r.URL.Path == "/v1/billing/purchases":
			purchaseRequests++
			writeAPIError(w, http.StatusInternalServerError, "server_error", "ambiguous purchase outcome")
		default:
			t.Fatalf("unexpected purchase replay request %s %s", r.Method, r.URL.Path)
		}
	})

	first := runManagement(t, state, server,
		"billing", "purchases", "create",
		"--package-id", "credits_1000",
		"--approval-proof-file", proofFile,
		"--idempotency-key", "idem-expired-purchase",
		"--yes",
		"--json",
	)
	if first.ExitCode != cli.ExitAPI || first.Err == nil {
		t.Fatalf("first purchase result = %#v", first)
	}

	opts := managementOptions()
	opts.Now = func() time.Time { return commandNow.Add(25 * time.Hour) }
	second := testutil.RunCommandWith(t, opts, state.APIArgs(server,
		"billing", "purchases", "create",
		"--package-id", "credits_1000",
		"--approval-proof-file", proofFile,
		"--idempotency-key", "idem-expired-purchase",
		"--yes",
		"--json",
	)...)
	if second.ExitCode == cli.ExitSuccess || !strings.Contains(second.Stderr, "24-hour idempotency replay window") {
		t.Fatalf("expired unknown purchase retry result = %#v", second)
	}
	if purchaseRequests != 1 {
		t.Fatalf("purchase POST count = %d, want 1", purchaseRequests)
	}
	if server.Count() != 3 {
		t.Fatalf("request count = %d, want 3", server.Count())
	}
}

func TestBillingPurchaseWaitFollowsPendingReconciliation(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	sleeper := &managementSleeper{}
	server := testutil.NewAPIServer(t, testutil.SequenceHandler(t,
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v1/billing/purchases/pur_wait" {
				t.Fatalf("unexpected first purchase poll %s %s", r.Method, r.URL.Path)
			}
			w.Header().Set("Retry-After", "11")
			writePurchaseEnvelope(t, w, "pur_wait", "pending", "req-purchase-pending")
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v1/billing/purchases/pur_wait" {
				t.Fatalf("unexpected second purchase poll %s %s", r.Method, r.URL.Path)
			}
			writePurchaseEnvelope(t, w, "pur_wait", "pending_reconciliation", "req-purchase-reconciliation")
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v1/billing/purchases/pur_wait" {
				t.Fatalf("unexpected final purchase poll %s %s", r.Method, r.URL.Path)
			}
			writePurchaseEnvelope(t, w, "pur_wait", "fulfilled", "req-purchase-fulfilled")
		},
	))

	opts := managementOptions()
	opts.Sleeper = sleeper
	result := testutil.RunCommandWith(t, opts, state.APIArgs(server, "billing", "purchases", "wait", "pur_wait", "--json")...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil {
		t.Fatalf("purchase wait result = %#v", result)
	}
	if server.Count() != 3 {
		t.Fatalf("request count = %d, want 3", server.Count())
	}
	if len(sleeper.waits) != 2 {
		t.Fatalf("wait count = %d, want 2", len(sleeper.waits))
	}
	if sleeper.waits[0] != 11*time.Second {
		t.Fatalf("first wait = %s, want 11s", sleeper.waits[0])
	}
	if !strings.Contains(result.Stdout, `"state": "fulfilled"`) {
		t.Fatalf("purchase wait output did not reach fulfilled:\n%s", result.Stdout)
	}
}

func TestTokenUpdateExpiryChangeRequiresConfirmation(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	proofFile := writeProofFixture(t, state, "approval-proof-token-expiry")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("expiry confirmation failure contacted API: %s %s", r.Method, r.URL.Path)
	})

	result := runManagement(t, state, server,
		"tokens", "update", "ak_target_public",
		"--revision", "3",
		"--expires-at-null",
		"--approval-proof-file", proofFile,
		"--no-prompt",
		"--json",
	)
	if result.ExitCode != cli.ExitUsage || !strings.Contains(result.Stderr, "confirmation") || !strings.Contains(result.Stderr, "--yes") {
		t.Fatalf("token expiry update missing-confirmation result = %#v", result)
	}
	if server.Count() != 0 {
		t.Fatalf("expiry confirmation failure contacted API; request count = %d", server.Count())
	}
}

func TestTokenRevokeSendsRevisionProofAndRequiresConfirmation(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	proof := "approval-proof-revoke"
	proofFile := writeProofFixture(t, state, proof)
	noConfirmServer := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("confirmation failure contacted API: %s %s", r.Method, r.URL.Path)
	})
	declined := runManagement(t, state, noConfirmServer,
		"tokens", "revoke", "ak_target_public",
		"--revision", "3",
		"--approval-proof-file", proofFile,
		"--json",
	)
	if declined.ExitCode != cli.ExitUsage || !strings.Contains(declined.Stderr, "confirmation") || !strings.Contains(declined.Stderr, "--yes") {
		t.Fatalf("token revoke missing-confirmation result = %#v", declined)
	}
	if noConfirmServer.Count() != 0 {
		t.Fatalf("confirmation failure contacted API; request count = %d", noConfirmServer.Count())
	}

	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/tokens/ak_target_public/revoke" {
			t.Fatalf("unexpected token revoke request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get(managementApprovalHeader) != proof {
			t.Fatal("token revoke approval proof header mismatch")
		}
		body := decodeObjectBody(t, r)
		assertBodyField(t, body, "expected_policy_revision", float64(3))
		if len(body) != 1 {
			t.Fatalf("token revoke body = %#v, want only expected_policy_revision", body)
		}
		testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"token": map[string]any{
					"token_public_id": "ak_target_public",
					"policy_revision": 4,
					"revoked":         true,
				},
			},
			"request_id": "req-token-revoke",
		})
	})
	result := runManagement(t, state, server,
		"tokens", "revoke", "ak_target_public",
		"--revision", "3",
		"--approval-proof-file", proofFile,
		"--idempotency-key", "idem-token-revoke",
		"--yes",
		"--json",
	)
	assertCommandSuccess(t, result)
	assertNotContains(t, result.Stdout+result.Stderr, proof)
	server.AssertIdempotencyKey(t, 0, "idem-token-revoke")
}

func TestApprovalWaitStopsWhenProofWasAlreadyIssued(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	approvalID := "map_already_issued"
	writeApprovalContextFixture(t, state, approvalID)
	proofOut := filepath.Join(state.Dir, "already-issued-proof.json")
	if privateOutputUnsupported() {
		server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("private-output refusal contacted API: %s %s", r.Method, r.URL.Path)
		})
		assertPrivateOutputRefusedBeforeHTTP(t, state, server, proofOut,
			"tokens", "approvals", "wait", approvalID,
			"--proof-out", proofOut,
			"--json",
		)
		return
	}
	server := testutil.NewAPIServer(t, testutil.SequenceHandler(t,
		func(w http.ResponseWriter, r *http.Request) {
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v1/management-approvals/"+approvalID {
				t.Fatalf("unexpected proof-issued poll %s %s", r.Method, r.URL.Path)
			}
			testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"approval_id":   approvalID,
					"state":         "proof_issued",
					"expires_at":    "2026-08-16T10:05:00+00:00",
					"next_check_at": nil,
				},
				"request_id": "req-proof-issued",
			})
		},
	))
	result := runManagement(t, state, server, "tokens", "approvals", "wait", approvalID, "--proof-out", proofOut, "--json")
	if result.ExitCode != cli.ExitUsage || !strings.Contains(result.Stderr, "fresh approval") {
		t.Fatalf("proof-issued result = %#v", result)
	}
	assertNotExists(t, proofOut)
	if server.Count() != 2 {
		t.Fatalf("request count = %d, want 2", server.Count())
	}
}

func TestWebhookSecretWorkflowsWritePrivateSecretsAndDoNotRetry(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	secretOut := filepath.Join(state.Dir, "webhook-secret.json")
	if privateOutputUnsupported() {
		server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("private-output refusal contacted API: %s %s", r.Method, r.URL.Path)
		})
		assertPrivateOutputRefusedBeforeHTTP(t, state, server, secretOut,
			"webhooks", "endpoints", "create",
			"--url", "https://hook.example.test/chab-events",
			"--event-type", "operation.succeeded",
			"--secret-out", secretOut,
			"--json",
		)
		return
	}
	signingSecret := testutil.FakeKey("webhook_signing_secret")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/webhooks/endpoints" {
			t.Fatalf("unexpected webhook create request %s %s", r.Method, r.URL.Path)
		}
		body := decodeObjectBody(t, r)
		assertBodyField(t, body, "url", "https://hook.example.test/chab-events")
		testutil.WriteJSON(t, w, http.StatusCreated, map[string]any{
			"data": map[string]any{
				"endpoint": map[string]any{"id": "whe_test_endpoint"},
				"secret":   signingSecret,
			},
			"request_id": "req-webhook-create",
		})
	})
	result := runManagement(t, state, server,
		"webhooks", "endpoints", "create",
		"--url", "https://hook.example.test/chab-events",
		"--event-type", "operation.succeeded",
		"--secret-out", secretOut,
		"--json",
	)
	assertCommandSuccess(t, result)
	assertNotContains(t, result.Stdout+result.Stderr, signingSecret)
	assertPrivateJSONFile(t, secretOut, func(raw string) {
		if !strings.Contains(raw, signingSecret) || !strings.Contains(raw, `"operation": "webhooks.endpoints.create"`) {
			t.Fatalf("webhook secret file missing expected private fields")
		}
	})
	server.AssertIdempotencyKeyMatches(t, 0, chabIdempotencyPattern())

	failState, _ := configuredChabCreditsState(t)
	failServer := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/webhooks/endpoints/whe_test_endpoint/rotate-secret" {
			t.Fatalf("unexpected webhook rotate request %s %s", r.Method, r.URL.Path)
		}
		writeAPIError(w, http.StatusInternalServerError, "server_error", "transient secret-route failure")
	})
	failed := runManagement(t, failState, failServer,
		"webhooks", "endpoints", "rotate-secret", "whe_test_endpoint",
		"--secret-out", filepath.Join(failState.Dir, "rotated-secret.json"),
		"--yes",
		"--json",
	)
	if failed.ExitCode != cli.ExitAPI || failed.Err == nil {
		t.Fatalf("webhook rotate failure result = %#v", failed)
	}
	if failServer.Count() != 1 {
		t.Fatalf("secret-producing webhook route was retried; request count = %d", failServer.Count())
	}
}

func TestWebhookSecretCommandsRemoveReservationWhenAuthFailsBeforeRequest(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "create",
			args: []string{
				"webhooks", "endpoints", "create",
				"--url", "https://hook.example.test/chab-events",
				"--event-type", "operation.succeeded",
				"--secret-out",
			},
		},
		{
			name: "rotate",
			args: []string{
				"webhooks", "endpoints", "rotate-secret", "whe_test_endpoint",
				"--yes",
				"--secret-out",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, _ := configuredChabCreditsState(t)
			secretOut := filepath.Join(state.Dir, tt.name+"-secret.json")
			args := append(append([]string{}, tt.args...), secretOut, "--json")
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("pre-request credential failure contacted API: %s %s", r.Method, r.URL.Path)
			})
			if privateOutputUnsupported() {
				assertPrivateOutputRefusedBeforeHTTP(t, state, server, secretOut, args...)
				return
			}
			if err := os.Remove(state.AuthPath); err != nil {
				t.Fatalf("remove auth fixture: %v", err)
			}

			result := runManagement(t, state, server, args...)
			if result.ExitCode == cli.ExitSuccess || !strings.Contains(result.Stderr, "CHAB_API_KEY") {
				t.Fatalf("webhook %s missing-auth result = %#v", tt.name, result)
			}
			assertNotExists(t, secretOut)
			if server.Count() != 0 {
				t.Fatalf("pre-request credential failure contacted API; request count = %d", server.Count())
			}
		})
	}
}

func TestManagementJSONBodyRejectsFieldFlagsBeforeHTTP(t *testing.T) {
	state, _ := configuredChabCreditsState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("body-mode validation contacted API: %s %s", r.Method, r.URL.Path)
	})
	result := runManagement(t, state, server,
		"webhooks", "endpoints", "create",
		"--body", `{"event_types":["operation.succeeded"],"url":"https://hook.example.test/chab-events"}`,
		"--url", "https://hook.example.test/ignored",
		"--dry-run",
		"--json",
	)
	if result.ExitCode != cli.ExitUsage || !strings.Contains(result.Stderr, "--body/--body-file cannot be combined with --url") {
		t.Fatalf("body-mode rejection result = %#v", result)
	}
	if server.Count() != 0 {
		t.Fatalf("body-mode rejection contacted API; request count = %d", server.Count())
	}
}

func privateOutputUnsupported() bool {
	return runtime.GOOS == "windows"
}

func assertPrivateOutputRefusedBeforeHTTP(t *testing.T, state testutil.State, server *testutil.APIServer, path string, args ...string) {
	t.Helper()
	result := runManagement(t, state, server, args...)
	if result.ExitCode != cli.ExitUsage || !strings.Contains(result.Stderr, "owner-only ACLs") {
		t.Fatalf("private-output refusal result = %#v", result)
	}
	assertNotExists(t, path)
	if server.Count() != 0 {
		t.Fatalf("private-output refusal contacted API; request count = %d", server.Count())
	}
}

func projectFixture(id any, description any, url any) map[string]any {
	return map[string]any{
		"id":          id,
		"name":        "Demo",
		"description": description,
		"url":         url,
		"status":      "active",
		"timezone":    "Europe/Berlin",
		"language":    "de",
		"limit":       100,
		"automate":    false,
		"created_at":  "2026-08-16T10:00:00+00:00",
		"updated_at":  "2026-08-16T10:00:00+00:00",
	}
}

func runManagement(t *testing.T, state testutil.State, server *testutil.APIServer, args ...string) testutil.Result {
	t.Helper()
	return testutil.RunCommandWith(t, managementOptions(), state.APIArgs(server, args...)...)
}

func managementOptions() testutil.Options {
	return testutil.Options{
		LookupEnv: testutil.HermeticEnv(nil),
		Now: func() time.Time {
			return commandNow
		},
	}
}

type managementSleeper struct {
	waits []time.Duration
}

func (s *managementSleeper) Sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.waits = append(s.waits, d)
	return nil
}

func writePurchaseEnvelope(t *testing.T, w http.ResponseWriter, id, state, requestID string) {
	t.Helper()
	testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":            id,
			"package_id":    "credits_1000",
			"state":         state,
			"next_check_at": nil,
			"created_at":    "2026-08-16T10:00:00+00:00",
		},
		"request_id": requestID,
	})
}

func decodeObjectBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("request body JSON error = %v", err)
	}
	return body
}

func assertBodyField(t *testing.T, body map[string]any, name string, want any) {
	t.Helper()
	got, ok := body[name]
	if !ok || got != want {
		t.Fatalf("body field %q = %#v, want %#v", name, got, want)
	}
}

func assertBodyNull(t *testing.T, body map[string]any, name string) {
	t.Helper()
	got, ok := body[name]
	if !ok || got != nil {
		t.Fatalf("body field %q = %#v, want JSON null", name, got)
	}
}

func firstPresentQuery(r *http.Request, keys ...string) string {
	for _, key := range keys {
		if _, ok := r.URL.Query()[key]; ok {
			return key
		}
	}
	return ""
}

func mustUnmarshal(t *testing.T, raw string, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		t.Fatalf("JSON decode error = %v: %s", err, raw)
	}
}

func assertCommandSuccess(t *testing.T, result testutil.Result) {
	t.Helper()
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("command result = %#v", result)
	}
}

func writeProofFixture(t *testing.T, state testutil.State, proof string) string {
	t.Helper()
	path := filepath.Join(state.Dir, proof+".json")
	payload, err := json.MarshalIndent(map[string]string{"proof": proof}, "", "  ")
	if err != nil {
		t.Fatalf("marshal proof fixture: %v", err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write proof fixture: %v", err)
	}
	return path
}

func writeApprovalContextFixture(t *testing.T, state testutil.State, approvalID string) {
	t.Helper()
	root := filepath.Join(filepath.Dir(state.ConfigPath), "management-approvals")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("create approval context dir: %v", err)
	}
	payload := map[string]any{
		"approval_id":              approvalID,
		"action":                   "tokens.revoke",
		"target_type":              "token",
		"target_id":                "ak_target_public",
		"expected_policy_revision": 3,
		"mutation":                 map[string]any{},
		"issuing_token_public_id":  "ak_01HY0000000000000000000000",
		"verification_url":         "https://chab.test/verify",
		"expires_at":               "2026-08-16T10:05:00+00:00",
		"next_check_at":            "2026-08-16T10:00:02+00:00",
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal approval context: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(root, approvalID+".json"), data, 0o600); err != nil {
		t.Fatalf("write approval context: %v", err)
	}
}

func assertPrivateJSONFile(t *testing.T, path string, inspect func(string)) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat private file: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("private file mode = %o, want 600", mode)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read private file: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("private file is not valid JSON")
	}
	inspect(string(raw))
}

func assertNotContains(t *testing.T, haystack string, forbidden ...string) {
	t.Helper()
	for _, value := range forbidden {
		if value != "" && strings.Contains(haystack, value) {
			t.Fatalf("output leaked a private value")
		}
	}
}

func assertPurchaseActionRecord(t *testing.T, state testutil.State, idem string) {
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
		OperationKey        string `json:"operation_key"`
		IdempotencyKey      string `json:"idempotency_key"`
		State               string `json:"state"`
		AcceptedOperationID string `json:"accepted_operation_id"`
		Path                string `json:"path"`
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("decode action record: %v", err)
	}
	if record.OperationKey != "billing.purchases.create" ||
		record.IdempotencyKey != idem ||
		record.State != "accepted" ||
		record.AcceptedOperationID != "pur_test_purchase" ||
		record.Path != "/v1/billing/purchases" {
		t.Fatalf("purchase action record = %#v", record)
	}
}

func singleActionID(t *testing.T, state testutil.State) string {
	t.Helper()
	actionDir := filepath.Join(filepath.Dir(state.ConfigPath), "actions")
	raw := readOnlyActionRecord(t, actionDir)
	var record struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		t.Fatalf("decode action record: %v", err)
	}
	if record.ID == "" {
		t.Fatal("action record missing id")
	}
	return record.ID
}
