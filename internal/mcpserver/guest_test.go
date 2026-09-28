package mcpserver_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestGuestLocalMCPAdvertisedToolsAndJournalOwnership(t *testing.T) {
	state := testutil.NewState(t)
	const guestKey = "chab_guest_dummy_test_credential"
	var searchCalls, emailCalls int
	var principalID atomic.Value
	principalID.Store("guest_trial:principal_one")
	var scopes atomic.Value
	scopes.Store([]string{"api:search:read", "api:contact:write", "api:credits:read"})
	var spendEnabled atomic.Bool
	spendEnabled.Store(true)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/v1/me":
			if r.Header.Get("Authorization") != "Bearer "+guestKey {
				t.Fatalf("unexpected Authorization header")
			}
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{
				"principal_type": "guest_trial", "principal_id": principalID.Load().(string),
				"guest_id": "guest_one", "scopes": scopes.Load().([]string),
			}))
		case "/v1/cli/compatibility":
			supported := []string{"auth.me", "credits.get", "operations.get", "operations.result", "operations.artifact", "operations.artifact_download", "operations.cancel"}
			guestTools := []map[string]any{}
			operationKeys := []string{}
			if spendEnabled.Load() {
				supported = append(supported, "search.web", "contacts.email_verify")
				operationKeys = []string{"search.web", "contacts.email_verify"}
				guestTools = []map[string]any{
					{"tool_name": "chab_search_web", "operation_key": "search.web", "scope": "api:search:read", "method": "POST", "path": "/v1/search/web", "idempotency_required": true, "funding_mode": "promotional_only", "credit_source": "guest_trial"},
					{"tool_name": "chab_contacts_email_verify", "operation_key": "contacts.email_verify", "scope": "api:contact:write", "method": "POST", "path": "/v1/contacts/email-verify", "idempotency_required": true, "funding_mode": "promotional_only", "credit_source": "guest_trial"},
				}
			}
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{
				"schema_version": "1.0.0", "api_major": 1, "minimum_version": "1.0.0",
				"recommended_version": "1.0.0", "catalog_version": "guest-test",
				"local_mcp": map[string]any{"transport": "stdio", "api_transport": "rest", "guest_trial_credentials": map[string]any{
					"accepted": true, "enabled": true, "spend_enabled": spendEnabled.Load(),
					"supported_route_operation_keys": supported,
					"operation_keys":                 operationKeys,
					"tools":                          guestTools,
				}},
			}))
		case "/v1/search/web":
			searchCalls++
			if r.Method != http.MethodPost || r.Header.Get("Idempotency-Key") == "" {
				t.Fatalf("guest search missing POST or idempotency key")
			}
			testutil.WriteJSON(t, w, http.StatusAccepted, testutil.Envelope(map[string]any{
				"id": "op_guest", "operation_key": "search.web", "family": "search", "status": "queued", "result_available": false,
			}))
		case "/v1/contacts/email-verify":
			emailCalls++
			if r.Method != http.MethodPost || r.Header.Get("Idempotency-Key") == "" {
				t.Fatalf("guest email verify missing POST or idempotency key")
			}
			testutil.WriteJSON(t, w, http.StatusAccepted, testutil.Envelope(map[string]any{"id": "op_email_guest", "operation_key": "contacts.email_verify", "family": "contacts", "status": "queued", "result_available": false}))
		case "/v1/operations/op_guest":
			if r.Method != http.MethodGet {
				t.Fatalf("guest status used %s", r.Method)
			}
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{"id": "op_guest", "operation_key": "search.web", "family": "search", "status": "succeeded", "result_available": true}))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", auth.ProfileAuth{APIKey: guestKey, PrincipalType: "guest_trial", PrincipalID: "guest_trial:principal_one"})
	h := startChabHarness(t, map[string]string{"CHAB_CONFIG": state.ConfigPath, "CHAB_AUTH_FILE": state.AuthPath, "CHAB_PROFILE": "local"})
	listed := responseResult(t, h.request("tools/list", map[string]any{}))
	tools := listed["tools"].([]any)
	var names []string
	for _, raw := range tools {
		names = append(names, raw.(map[string]any)["name"].(string))
	}
	for _, name := range []string{"chab_search_web", "chab_contacts_email_verify", "chab_auth_me", "chab_credits_get", "chab_operations_cancel", "chab_action_resume"} {
		if !containsStringString(names, name) {
			t.Fatalf("guest tools missing %s: %v", name, names)
		}
	}
	for _, name := range []string{"chab_tokens_update", "chab_billing_packages_list", "chab_operations_list", "chab_projects_list"} {
		if containsStringString(names, name) {
			t.Fatalf("guest tools must hide %s: %v", name, names)
		}
	}
	denied := responseResult(t, h.callTool("chab_tokens_update", map[string]any{}))
	if denied["isError"] != true || !strings.Contains(denied["structuredContent"].(map[string]any)["message"].(string), "not available") {
		t.Fatalf("guest token-management denial = %#v", denied)
	}
	started := responseResult(t, h.callTool("chab_search_web", map[string]any{
		"body":  map[string]any{"query": "coffee", "engine": "google", "location": map[string]any{"country": "US"}, "language": "en"},
		"local": map[string]any{"confirmation": true},
	}))
	if started["isError"] == true || searchCalls != 1 {
		t.Fatalf("guest search start = %#v, calls=%d", started, searchCalls)
	}
	assertGuestToolResultSchema(t, toolByName(t, tools, "chab_search_web"), started)
	email := responseResult(t, h.callTool("chab_contacts_email_verify", map[string]any{"body": map[string]any{"email": "ada@example.com"}, "local": map[string]any{"confirmation": true}}))
	if email["isError"] == true || emailCalls != 1 {
		t.Fatalf("guest email verify = %#v, calls=%d", email, emailCalls)
	}
	assertGuestToolResultSchema(t, toolByName(t, tools, "chab_contacts_email_verify"), email)
	scopes.Store([]string{"api:credits:read"})
	limited := responseResult(t, h.request("tools/list", map[string]any{}))["tools"].([]any)
	for _, raw := range limited {
		name := raw.(map[string]any)["name"].(string)
		if name == "chab_search_web" || name == "chab_contacts_email_verify" {
			t.Fatalf("limited scope advertised %s", name)
		}
	}
	limitedCall := responseResult(t, h.callTool("chab_search_web", map[string]any{}))
	if limitedCall["isError"] != true || searchCalls != 1 {
		t.Fatalf("limited scope started work: %#v", limitedCall)
	}
	scopes.Store([]string{"api:search:read", "api:contact:write", "api:credits:read"})
	action := started["structuredContent"].(map[string]any)["action"].(map[string]any)
	if action["principal_id"] != "guest_trial:principal_one" || action["token_public_id"] != nil {
		t.Fatalf("guest action owner = %#v", action)
	}
	ownedList := responseResult(t, h.callTool("chab_action_list", map[string]any{}))
	assertGuestToolResultSchema(t, toolByName(t, tools, "chab_action_list"), ownedList)
	ownedShow := responseResult(t, h.callTool("chab_action_show", map[string]any{"action_id": action["id"]}))
	assertGuestToolResultSchema(t, toolByName(t, tools, "chab_action_show"), ownedShow)
	spendEnabled.Store(false)
	pausedTools := responseResult(t, h.request("tools/list", map[string]any{}))["tools"].([]any)
	var pausedNames []string
	for _, raw := range pausedTools {
		pausedNames = append(pausedNames, raw.(map[string]any)["name"].(string))
	}
	if containsStringString(pausedNames, "chab_search_web") || !containsStringString(pausedNames, "chab_operations_get") || !containsStringString(pausedNames, "chab_action_resume") {
		t.Fatalf("paused guest tools = %v", pausedNames)
	}
	status := responseResult(t, h.callTool("chab_operations_get", map[string]any{"path": map[string]any{"id": "op_guest"}}))
	if status["isError"] == true {
		t.Fatalf("paused guest status denied: %#v", status)
	}
	acceptedResume := responseResult(t, h.callTool("chab_action_resume", map[string]any{"action_id": action["id"]}))
	if acceptedResume["isError"] == true || acceptedResume["structuredContent"].(map[string]any)["operation_id"] != "op_guest" {
		t.Fatalf("paused accepted action cannot resume: %#v", acceptedResume)
	}
	spendEnabled.Store(true)
	principalID.Store("guest_trial:principal_two")
	resume := responseResult(t, h.callTool("chab_action_resume", map[string]any{"action_id": action["id"]}))
	if resume["isError"] != true || searchCalls != 1 {
		t.Fatalf("rotated guest must not resume old action: %#v calls=%d", resume, searchCalls)
	}
	shown := responseResult(t, h.callTool("chab_action_show", map[string]any{"action_id": action["id"]}))
	if shown["isError"] != true {
		t.Fatalf("rotated guest must not see old action: %#v", shown)
	}
	list := responseResult(t, h.callTool("chab_action_list", map[string]any{}))
	if len(list["structuredContent"].([]any)) != 0 {
		t.Fatalf("rotated guest action list leaked prior owner: %#v", list)
	}
	authFile, _, err := auth.Load(state.AuthPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.DeleteProfile(authFile, "local"); err != nil {
		t.Fatal(err)
	}
	if err := auth.Write(state.AuthPath, authFile); err != nil {
		t.Fatal(err)
	}
	withoutCredential := responseResult(t, h.callTool("chab_action_show", map[string]any{"action_id": action["id"]}))
	if withoutCredential["isError"] != true {
		t.Fatalf("logged-out guest receipt leaked: %#v", withoutCredential)
	}
	withoutList := responseResult(t, h.callTool("chab_action_list", map[string]any{}))
	if len(withoutList["structuredContent"].([]any)) != 0 {
		t.Fatalf("logged-out guest action list leaked: %#v", withoutList)
	}
}

func TestGuestLocalMCPLifecycleErrorsKeepBackendCodes(t *testing.T) {
	for _, code := range []string{
		"free_credits_exhausted", "free_usage_paused", "challenge_required",
		"guest_credential_expired", "guest_credential_revoked", "signup_required",
	} {
		t.Run(code, func(t *testing.T) {
			state := testutil.NewState(t)
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.EscapedPath() != "/v1/me" {
					t.Fatalf("unexpected request %s", r.URL.EscapedPath())
				}
				value := testutil.ErrorEnvelope(code, "guest action unavailable", "req_guest_denial", nil)
				value["error"].(map[string]any)["retryable"] = code == "challenge_required" || code == "free_usage_paused"
				testutil.WriteJSON(t, w, http.StatusForbidden, value)
			})
			testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
			testutil.WriteAuthProfile(t, state.AuthPath, "local", auth.ProfileAuth{APIKey: "chab_guest_dummy_lifecycle", PrincipalType: "guest_trial"})
			h := startChabHarness(t, map[string]string{"CHAB_CONFIG": state.ConfigPath, "CHAB_AUTH_FILE": state.AuthPath, "CHAB_PROFILE": "local"})
			result := responseResult(t, h.callTool("chab_credits_get", map[string]any{}))
			if result["isError"] != true {
				t.Fatalf("expected guest denial: %#v", result)
			}
			structured := result["structuredContent"].(map[string]any)
			if structured["code"] != code || structured["request_id"] != "req_guest_denial" || structured["retryable"] != (code == "challenge_required" || code == "free_usage_paused") {
				t.Fatalf("guest denial lost API context: %#v", structured)
			}
		})
	}
}

func TestGuestDeniedStartCannotResumeAsNewWork(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		code    string
		outcome string
	}{
		{"free_credits_exhausted", http.StatusPaymentRequired, "free_credits_exhausted", "released"},
		{"free_usage_paused", http.StatusTooManyRequests, "free_usage_paused", "released"},
		{"rate_limited_released", http.StatusTooManyRequests, "rate_limited", "released"},
		{"rate_limited_settled", http.StatusTooManyRequests, "rate_limited", "settled"},
		{"promotional_budget_released", http.StatusServiceUnavailable, "promotional_budget_exhausted", "released"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := testutil.NewState(t)
			starts := 0
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.EscapedPath() {
				case "/v1/me":
					testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{"principal_type": "guest_trial", "principal_id": "guest_trial:denied", "guest_id": "denied", "scopes": []string{"api:search:read"}}))
				case "/v1/cli/compatibility":
					testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{"schema_version": "1.0.0", "api_major": 1, "minimum_version": "1.0.0", "recommended_version": "1.0.0", "catalog_version": "denial-test", "local_mcp": map[string]any{"transport": "stdio", "api_transport": "rest", "guest_trial_credentials": map[string]any{"accepted": false, "enabled": false, "spend_enabled": true, "supported_route_operation_keys": []string{"search.web"}, "tools": []map[string]any{{"tool_name": "chab_search_web", "operation_key": "search.web", "scope": "api:search:read", "method": "POST", "path": "/v1/search/web", "idempotency_required": true, "funding_mode": "promotional_only", "credit_source": "guest_trial"}}}}}))
				case "/v1/search/web":
					starts++
					w.Header().Set("X-Chab-Idempotency-Outcome", tc.outcome)
					if tc.status == http.StatusTooManyRequests {
						w.Header().Set("Retry-After", "1")
					}
					value := testutil.ErrorEnvelope(tc.code, "free start denied", "req_denied", nil)
					value["error"].(map[string]any)["user_action"] = "Sign up or add credits."
					testutil.WriteJSON(t, w, tc.status, value)
				default:
					t.Fatalf("unexpected request %s", r.URL.EscapedPath())
				}
			})
			testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
			testutil.WriteAuthProfile(t, state.AuthPath, "local", auth.ProfileAuth{APIKey: "chab_guest_dummy_denied", PrincipalType: "guest_trial", PrincipalID: "guest_trial:denied"})
			h := startChabHarness(t, map[string]string{"CHAB_CONFIG": state.ConfigPath, "CHAB_AUTH_FILE": state.AuthPath, "CHAB_PROFILE": "local"})
			started := responseResult(t, h.callTool("chab_search_web", map[string]any{"body": map[string]any{"query": "coffee", "engine": "google", "location": map[string]any{"country": "US"}, "language": "en"}, "local": map[string]any{"confirmation": true}}))
			if started["isError"] != true || starts != 1 {
				t.Fatalf("denied start = %#v, starts=%d", started, starts)
			}
			value := started["structuredContent"].(map[string]any)
			if value["error"].(map[string]any)["user_action"] != "Sign up or add credits." {
				t.Fatalf("lost remediation: %#v", value)
			}
			listed := responseResult(t, h.request("tools/list", map[string]any{}))["tools"].([]any)
			assertGuestToolResultSchema(t, toolByName(t, listed, "chab_search_web"), started)
			action := value["action"].(map[string]any)
			if action["state"] != "denied" || value["local_recovery"].(map[string]any)["can_resume"] != false {
				t.Fatalf("denial marked replayable: %#v", value)
			}
			resumed := responseResult(t, h.callTool("chab_action_resume", map[string]any{"action_id": action["id"], "body": map[string]any{"query": "coffee", "engine": "google", "location": map[string]any{"country": "US"}, "language": "en"}, "local": map[string]any{"confirmation": true}}))
			if resumed["isError"] != true || starts != 1 {
				t.Fatalf("denied action replayed: %#v starts=%d", resumed, starts)
			}
		})
	}
}

func assertGuestToolResultSchema(t *testing.T, tool map[string]any, result map[string]any) {
	t.Helper()
	raw, err := json.Marshal(tool["outputSchema"])
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := resolved.Validate(result["structuredContent"]); err != nil {
		t.Fatalf("%s result violates advertised output schema: %v", tool["name"], err)
	}
}

func TestGuestUnknownStartRetainsRecoveryAcrossTemporaryReplayFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
	}{
		{"idempotency_in_progress", http.StatusConflict, "idempotency_request_in_progress"},
		{"rate_limited", http.StatusTooManyRequests, "rate_limited"},
		{"preclaim_scope_denial", http.StatusForbidden, "api_scope_missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := testutil.NewState(t)
			starts := 0
			var firstKey string
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.EscapedPath() {
				case "/v1/me":
					testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{"principal_type": "guest_trial", "principal_id": "guest_trial:recovery", "guest_id": "recovery", "scopes": []string{"api:search:read"}}))
				case "/v1/cli/compatibility":
					testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{"schema_version": "1.0.0", "api_major": 1, "minimum_version": "1.0.0", "recommended_version": "1.0.0", "catalog_version": "recovery-test", "local_mcp": map[string]any{"transport": "stdio", "api_transport": "rest", "guest_trial_credentials": map[string]any{"accepted": true, "enabled": true, "spend_enabled": true, "supported_route_operation_keys": []string{"search.web"}, "tools": []map[string]any{{"tool_name": "chab_search_web", "operation_key": "search.web", "scope": "api:search:read", "method": "POST", "path": "/v1/search/web", "idempotency_required": true, "funding_mode": "promotional_only", "credit_source": "guest_trial"}}}}}))
				case "/v1/search/web":
					starts++
					key := r.Header.Get("Idempotency-Key")
					if key == "" || (firstKey != "" && firstKey != key) {
						t.Fatalf("recovery changed idempotency key: %q, first %q", key, firstKey)
					}
					firstKey = key
					if starts == 1 {
						testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("server_error", "uncertain", "req_uncertain", nil))
						return
					}
					if starts == 2 {
						if tc.status == http.StatusTooManyRequests {
							w.Header().Set("Retry-After", "1")
						}
						testutil.WriteJSON(t, w, tc.status, testutil.ErrorEnvelope(tc.code, "try again later", "req_retry_later", nil))
						return
					}
					testutil.WriteJSON(t, w, http.StatusAccepted, testutil.Envelope(map[string]any{"id": "op_recovered", "operation_key": "search.web", "family": "search", "status": "queued", "result_available": false}))
				default:
					t.Fatalf("unexpected request %s", r.URL.EscapedPath())
				}
			})
			testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
			testutil.WriteAuthProfile(t, state.AuthPath, "local", auth.ProfileAuth{APIKey: "chab_guest_dummy_recovery", PrincipalType: "guest_trial", PrincipalID: "guest_trial:recovery"})
			h := startChabHarness(t, map[string]string{"CHAB_CONFIG": state.ConfigPath, "CHAB_AUTH_FILE": state.AuthPath, "CHAB_PROFILE": "local"})
			body := map[string]any{"query": "coffee", "engine": "google", "location": map[string]any{"country": "US"}, "language": "en"}
			first := responseResult(t, h.callTool("chab_search_web", map[string]any{"body": body, "local": map[string]any{"confirmation": true}}))
			if first["isError"] != true {
				t.Fatalf("first start should be uncertain: %#v", first)
			}
			action := first["structuredContent"].(map[string]any)["action"].(map[string]any)
			resumeInput := map[string]any{"action_id": action["id"], "body": body, "local": map[string]any{"confirmation": true}}
			second := responseResult(t, h.callTool("chab_action_resume", resumeInput))
			if second["isError"] != true {
				t.Fatalf("temporary replay failure expected: %#v", second)
			}
			secondValue := second["structuredContent"].(map[string]any)
			if secondValue["local_recovery"].(map[string]any)["can_resume"] != true || secondValue["action"].(map[string]any)["state"] != "unknown" {
				t.Fatalf("temporary replay failure lost recovery: %#v", secondValue)
			}
			third := responseResult(t, h.callTool("chab_action_resume", resumeInput))
			if third["isError"] == true || starts != 3 {
				t.Fatalf("third resume failed: %#v starts=%d", third, starts)
			}
		})
	}
}

func TestGuestLocalMCPRejectsAdvertisedRouteDrift(t *testing.T) {
	state := testutil.NewState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/v1/me":
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{"principal_type": "guest_trial", "principal_id": "guest_trial:one", "guest_id": "gt_one"}))
		case "/v1/cli/compatibility":
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{
				"schema_version": "1.0.0", "api_major": 1, "minimum_version": "1.0.0", "recommended_version": "1.0.0", "catalog_version": "drift-test",
				"local_mcp": map[string]any{"transport": "stdio", "api_transport": "rest", "guest_trial_credentials": map[string]any{
					"accepted": true, "enabled": true, "spend_enabled": true, "supported_route_operation_keys": []string{"search.web"},
					"tools": []map[string]any{{"tool_name": "chab_search_web", "operation_key": "search.web", "scope": "api:search:read", "method": "POST", "path": "/v1/tokens", "idempotency_required": true, "funding_mode": "promotional_only", "credit_source": "guest_trial"}},
				}},
			}))
		default:
			t.Fatalf("guest drift must not call %s", r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", auth.ProfileAuth{APIKey: "chab_guest_dummy_drift", PrincipalType: "guest_trial"})
	h := startChabHarness(t, map[string]string{"CHAB_CONFIG": state.ConfigPath, "CHAB_AUTH_FILE": state.AuthPath, "CHAB_PROFILE": "local"})
	result := responseResult(t, h.callTool("chab_search_web", map[string]any{}))
	if result["isError"] != true {
		t.Fatalf("route drift did not fail closed: %#v", result)
	}
	listed := responseResult(t, h.request("tools/list", map[string]any{}))
	for _, raw := range listed["tools"].([]any) {
		if raw.(map[string]any)["name"] == "chab_search_web" {
			t.Fatalf("route drift advertised guest start: %#v", listed)
		}
	}
}
