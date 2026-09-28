package mcpserver_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/mcpserver"
	"github.com/vincentsch/chab-cli/internal/redact"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestChabToolsListWireShape(t *testing.T) {
	h := startChabHarness(t, nil)
	result := responseResult(t, h.request("tools/list", map[string]any{}))
	tools := result["tools"].([]any)
	var names []string
	for _, tool := range tools {
		row := tool.(map[string]any)
		names = append(names, row["name"].(string))
		if row["inputSchema"] == nil || row["outputSchema"] == nil || row["annotations"] == nil {
			t.Fatalf("tool missing schema or annotations: %#v", row)
		}
	}
	registry, err := chabcontract.Load()
	if err != nil {
		t.Fatal(err)
	}
	eligibleOperations := 0
	for _, op := range registry.Operations() {
		switch op.ID {
		case "cli.compatibility", "billing.auto_recharge.update", "billing.purchases.create",
			"tokens.create", "tokens.management_approvals.get",
			"webhooks.endpoints.create", "webhooks.endpoints.rotate_secret":
			continue
		default:
			eligibleOperations++
		}
	}
	if got, want := len(names), eligibleOperations+4; got != want {
		t.Fatalf("tool count = %d, want %d; names=%v", got, want, names)
	}
	for _, want := range []string{
		"chab_auth_env",
		"chab_action_list",
		"chab_action_show",
		"chab_action_resume",
		"chab_auth_me",
		"chab_credits_get",
		"chab_credits_transactions_list",
		"chab_health",
		"chab_errors",
		"chab_search_web",
		"chab_files_create",
		"chab_files_download",
		"chab_tokens_update",
	} {
		if !containsStringString(names, want) {
			t.Fatalf("tool names missing %s: %v", want, names)
		}
	}
	for _, forbidden := range []string{
		"chab_whoami",
		"chab_credits_balance",
		"chab_credits_transactions",
		"chab_tokens_create",
		"chab_tokens_management_approvals_get",
		"chab_webhooks_endpoints_rotate_secret",
		"chab_billing_purchases_create",
	} {
		if containsStringString(names, forbidden) {
			t.Fatalf("tool names contain forbidden %s: %v", forbidden, names)
		}
	}
}

func TestChabAuthEnvIsLocalOnlyWithoutCredentials(t *testing.T) {
	server := testutil.NewAPIServer(t, testutil.FailOnContact(t))
	state := testutil.NewState(t)
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":       state.ConfigPath,
		"CHAB_AUTH_FILE":    state.AuthPath,
		"CHAB_PROFILE":      "local",
		"CHAB_BASE_URL":     server.URL,
		"CHAB_API_BASE_URL": server.APIBaseURL(),
		"CHAB_LOCALE":       "en",
	})

	result := responseResult(t, h.callTool("chab_auth_env", map[string]any{}))
	if result["isError"] == true {
		t.Fatalf("auth env isError: %#v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	if structured["secret_variable"] != "CHAB_API_KEY" || structured["api_base_url"] != server.APIBaseURL() {
		t.Fatalf("auth env structuredContent = %#v", structured)
	}
	server.AssertNoRequests(t)
}

func TestChabPublicToolsDoNotRequireCredentials(t *testing.T) {
	state := testutil.NewState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/v1/health":
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{
				"service": "chab", "api_version": "v1", "status": "ok", "updated_at": "2026-09-24T17:00:00Z",
				"families": []map[string]any{{
					"family": "search", "state": "healthy", "covered_operation_keys": []string{"search.web"}, "remediation": nil,
				}},
				"free_mode": map[string]any{"state": "enabled"},
			}))
		case "/v1/errors":
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{"errors": []map[string]any{{"code": "invalid_api_token", "http_status": 401}}}))
		default:
			t.Fatalf("unexpected public MCP request path %s", r.URL.EscapedPath())
		}
	})
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":       state.ConfigPath,
		"CHAB_AUTH_FILE":    state.AuthPath,
		"CHAB_PROFILE":      "local",
		"CHAB_API_BASE_URL": server.APIBaseURL(),
	})

	health := responseResult(t, h.callTool("chab_health", map[string]any{}))["structuredContent"].(map[string]any)
	if health["status"] != "ok" {
		t.Fatalf("health structuredContent = %#v", health)
	}
	if health["free_mode"].(map[string]any)["state"] != "enabled" ||
		health["families"].([]any)[0].(map[string]any)["covered_operation_keys"].([]any)[0] != "search.web" {
		t.Fatalf("health coverage lost in structuredContent = %#v", health)
	}
	tools := responseResult(t, h.request("tools/list", map[string]any{}))["tools"].([]any)
	var healthSchema map[string]any
	for _, tool := range tools {
		row := tool.(map[string]any)
		if row["name"] == "chab_health" {
			for _, variant := range row["outputSchema"].(map[string]any)["oneOf"].([]any) {
				candidate := variant.(map[string]any)
				if candidate["properties"].(map[string]any)["families"] != nil {
					healthSchema = candidate
					break
				}
			}
			break
		}
	}
	if healthSchema == nil || !containsStringString(requiredStrings(healthSchema), "free_mode") {
		t.Fatalf("chab_health output schema lacks required free_mode: %#v", healthSchema)
	}
	properties := healthSchema["properties"].(map[string]any)
	if !containsStringString(requiredStrings(properties["free_mode"].(map[string]any)), "state") {
		t.Fatalf("chab_health free_mode schema lacks state: %#v", properties["free_mode"])
	}
	familySchema := properties["families"].(map[string]any)["items"].(map[string]any)
	if !containsStringString(requiredStrings(familySchema), "covered_operation_keys") ||
		familySchema["properties"].(map[string]any)["covered_operation_keys"] == nil {
		t.Fatalf("chab_health output schema lacks covered_operation_keys: %#v", familySchema)
	}
	coverageSchema := familySchema["properties"].(map[string]any)["covered_operation_keys"].(map[string]any)
	var arrayAllowed, nullAllowed bool
	for _, variant := range coverageSchema["oneOf"].([]any) {
		switch variant.(map[string]any)["type"] {
		case "array":
			arrayAllowed = variant.(map[string]any)["items"].(map[string]any)["type"] == "string"
		case "null":
			nullAllowed = true
		}
	}
	if !arrayAllowed || !nullAllowed {
		t.Fatalf("chab_health coverage schema must allow string-array or null: %#v", coverageSchema)
	}
	errors := responseResult(t, h.callTool("chab_errors", map[string]any{}))["structuredContent"].(map[string]any)
	if rows := errors["errors"].([]any); rows[0].(map[string]any)["code"] != "invalid_api_token" {
		t.Fatalf("errors structuredContent = %#v", errors)
	}
	for i := range server.Requests() {
		if got := server.Request(t, i).Header.Get("Authorization"); got != "" {
			t.Fatalf("public request %d Authorization header = %q, want absent", i, got)
		}
	}
}

func TestChabWhoamiMCPReturnsFlatIdentity(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_whoami")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/v1/me" {
			t.Fatalf("unexpected MCP whoami request %s %s", r.Method, r.URL.EscapedPath())
		}
		testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{
			"principal_type":  "team_api_token",
			"team_id":         42,
			"token_id":        "tok_internal",
			"token_public_id": "870b679b-210c-4d33-9d2c-a5b091693f42",
			"scopes":          []string{"api:credits:read"},
			"token_controls": map[string]any{
				"policy_revision": 7,
				"feature_access":  map[string]any{"mode": "selected", "snapshot_stale": false},
				"ip_restrictions": map[string]any{"restricted": false, "allow_rule_count": 0, "deny_rule_count": 0},
				"spending": map[string]any{
					"mode":                    "enabled",
					"active_reserved_credits": 0,
					"allowance":               nil,
				},
				"project_access": map[string]any{"mode": "all", "selected_project_ids": []int{1, 2}, "selected_count": 2},
			},
			"request_id": "req-whoami",
		}))
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	tools := responseResult(t, h.request("tools/list", map[string]any{}))["tools"].([]any)
	var whoamiSchema map[string]any
	for _, tool := range tools {
		row := tool.(map[string]any)
		if row["name"] == "chab_auth_me" {
			whoamiSchema = row["outputSchema"].(map[string]any)
			break
		}
	}
	if whoamiSchema == nil {
		t.Fatal("chab_auth_me schema not found")
	}
	oneOf := whoamiSchema["oneOf"].([]any)
	successSchema := oneOf[0].(map[string]any)
	props := successSchema["properties"].(map[string]any)
	if props["server"] == nil || props["request_id"] == nil {
		t.Fatalf("whoami output schema properties = %#v", props)
	}

	result := responseResult(t, h.callTool("chab_auth_me", map[string]any{}))
	if result["isError"] == true {
		t.Fatalf("whoami returned error: %#v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	serverContent := structured["server"].(map[string]any)
	if structured["request_id"] != "req-header" || serverContent["request_id"] != "req-whoami" || serverContent["principal_type"] != "team_api_token" || serverContent["team_id"] != float64(42) || serverContent["token_public_id"] != "870b679b-210c-4d33-9d2c-a5b091693f42" {
		t.Fatalf("whoami structuredContent = %#v", structured)
	}
	if _, ok := serverContent["team"]; ok {
		t.Fatalf("whoami returned legacy team object: %#v", structured)
	}
	server.AssertBearer(t, key)
}

func TestChabMCPProjectReadRedactsRegisteredCredential(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_project_read")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/v1/projects/42" {
			t.Fatalf("unexpected MCP project request %s %s", r.Method, r.URL.EscapedPath())
		}
		testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"project": map[string]any{
					"id":          42,
					"name":        key,
					"description": key,
					"url":         nil,
					"status":      "active",
					"timezone":    "Europe/Berlin",
					"language":    "de",
					"limit":       100,
					"automate":    false,
					"created_at":  "2026-08-16T10:00:00+00:00",
					"updated_at":  "2026-08-16T10:00:00+00:00",
				},
			},
			"request_id": "req-project-read-redact",
		})
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	result := responseResult(t, h.callTool("chab_projects_get", map[string]any{
		"path": map[string]any{"project_id": 42},
	}))
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), key) {
		t.Fatalf("MCP project read leaked registered credential: %s", encoded)
	}
	content := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if strings.Contains(content, key) {
		t.Fatalf("MCP text content leaked registered credential: %s", content)
	}
	serverContent := result["structuredContent"].(map[string]any)["server"].(map[string]any)
	project := serverContent["project"].(map[string]any)
	if project["name"] != "[REDACTED]" || project["description"] != "[REDACTED]" {
		t.Fatalf("MCP project redaction = %#v", project)
	}
	server.AssertBearer(t, key)
}

func TestChabMCPActionSuccessRedactsRegisteredSecrets(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_project_create")
	idem := "idem-mcp-project-create"
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/me":
			writeMCPWhoami(t, w)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/projects":
			if got := r.Header.Get("Idempotency-Key"); got != idem {
				t.Fatalf("idempotency key = %q, want %q", got, idem)
			}
			testutil.WriteJSON(t, w, http.StatusCreated, testutil.Envelope(map[string]any{
				"project": map[string]any{
					"id":          42,
					"name":        key,
					"description": idem,
				},
				"echo": map[string]any{
					"credential":  key,
					"idempotency": idem,
				},
			}))
		default:
			t.Fatalf("unexpected MCP project create request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	result := responseResult(t, h.callTool("chab_projects_create", map[string]any{
		"body":  map[string]any{"name": "Demo"},
		"local": map[string]any{"idempotency_key": idem},
	}))
	if result["isError"] == true {
		t.Fatalf("project create returned error: %#v", result)
	}
	assertMCPResultNotContains(t, result, key, idem)
	serverContent := result["structuredContent"].(map[string]any)["server"].(map[string]any)
	project := serverContent["project"].(map[string]any)
	echo := serverContent["echo"].(map[string]any)
	if project["name"] != "[REDACTED]" || project["description"] != "[REDACTED]" || echo["credential"] != "[REDACTED]" || echo["idempotency"] != "[REDACTED]" {
		t.Fatalf("project create redaction = %#v", serverContent)
	}
	server.AssertBearer(t, key)
}

func TestChabMCPActionPartialResultRedactsRegisteredSecrets(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_project_partial")
	idem := "idem-mcp-project-partial"
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/me":
			writeMCPWhoami(t, w)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/projects":
			if got := r.Header.Get("Idempotency-Key"); got != idem {
				t.Fatalf("idempotency key = %q, want %q", got, idem)
			}
			testutil.WriteJSON(t, w, http.StatusAccepted, testutil.Envelope(map[string]any{
				"id":               "op_partial",
				"operation_key":    "projects.create",
				"status":           "mystery",
				"echo_credential":  key,
				"echo_idempotency": idem,
			}))
		default:
			t.Fatalf("unexpected MCP project partial request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	result := responseResult(t, h.callTool("chab_projects_create", map[string]any{
		"body":  map[string]any{"name": "Demo"},
		"local": map[string]any{"idempotency_key": idem},
	}))
	if result["isError"] != true {
		t.Fatalf("partial result should be a tool error: %#v", result)
	}
	assertMCPResultNotContains(t, result, key, idem)
	serverContent := result["structuredContent"].(map[string]any)["server"].(map[string]any)
	if serverContent["echo_credential"] != "[REDACTED]" || serverContent["echo_idempotency"] != "[REDACTED]" {
		t.Fatalf("partial result redaction = %#v", serverContent)
	}
	server.AssertBearer(t, key)
}

func TestChabMCPAPIErrorPreservesChabContract(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_error")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/v1/credits" {
			t.Fatalf("unexpected MCP credits request %s %s", r.Method, r.URL.EscapedPath())
		}
		testutil.WriteJSON(t, w, http.StatusForbidden, map[string]any{
			"error": map[string]any{
				"code":      "api_scope_missing",
				"message":   "The API token is missing the scope required for this operation.",
				"retryable": false,
				"details": map[string]any{
					"required_scope": "api:credits:read",
					"nested":         map[string]any{"reason": "scope"},
				},
			},
			"request_id": "req-scope",
		})
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	result := responseResult(t, h.callTool("chab_credits_get", map[string]any{}))
	if result["isError"] != true {
		t.Fatalf("credits balance did not return tool error: %#v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	if structured["code"] != "api_scope_missing" || structured["retryable"] != false || structured["request_id"] != "req-scope" || structured["http_status"] != float64(http.StatusForbidden) {
		t.Fatalf("tool error structuredContent = %#v", structured)
	}
	details := structured["details"].(map[string]any)
	if details["required_scope"] != "api:credits:read" || details["nested"].(map[string]any)["reason"] != "scope" {
		t.Fatalf("tool error details = %#v", details)
	}
	server.AssertBearer(t, key)
}

func TestChabOperationToolSchemaGroupsRouteAndLocalControls(t *testing.T) {
	h := startChabHarness(t, nil)
	tools := responseResult(t, h.request("tools/list", map[string]any{}))["tools"].([]any)
	download := toolByName(t, tools, "chab_drive_items_download")
	input := download["inputSchema"].(map[string]any)
	props := input["properties"].(map[string]any)
	for _, group := range []string{"path", "query", "local"} {
		if props[group] == nil {
			t.Fatalf("download input missing %s group: %#v", group, input)
		}
	}
	pathRequired := props["path"].(map[string]any)["required"].([]any)
	if !containsString(pathRequired, "item_id") {
		t.Fatalf("path required = %#v, want item_id", pathRequired)
	}
	localRequired := props["local"].(map[string]any)["required"].([]any)
	if !containsString(localRequired, "output_path") {
		t.Fatalf("local required = %#v, want output_path", localRequired)
	}

	search := toolByName(t, tools, "chab_search_web")
	annotations := search["annotations"].(map[string]any)
	if annotations["readOnlyHint"] != false {
		t.Fatalf("paid search readOnlyHint = %#v, want false", annotations["readOnlyHint"])
	}
	searchInput := search["inputSchema"].(map[string]any)
	searchProps := searchInput["properties"].(map[string]any)
	if searchProps["body"] == nil || searchProps["local"] == nil {
		t.Fatalf("search input missing body/local groups: %#v", searchInput)
	}

	tokenUpdate := toolByName(t, tools, "chab_tokens_update")
	tokenLocal := tokenUpdate["inputSchema"].(map[string]any)["properties"].(map[string]any)["local"].(map[string]any)
	tokenLocalProps := tokenLocal["properties"].(map[string]any)
	if tokenLocalProps["approval_proof_path"] == nil || tokenLocalProps["confirmation"] == nil {
		t.Fatalf("token update local controls = %#v, want approval proof and confirmation", tokenLocal)
	}
	if !containsString(tokenLocal["required"].([]any), "confirmation") {
		t.Fatalf("token update local required = %#v, want confirmation", tokenLocal["required"])
	}
}

func TestChabPaidMCPToolRequiresConfirmationBeforeNetwork(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_paid")
	server := testutil.NewAPIServer(t, testutil.FailOnContact(t))
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	result := responseResult(t, h.callTool("chab_search_web", map[string]any{
		"body": map[string]any{
			"query":    "coffee",
			"engine":   "google",
			"location": map[string]any{"country": "US"},
			"language": "en",
		},
	}))
	if result["isError"] != true {
		t.Fatalf("search without confirmation should be tool error: %#v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	if structured["code"] != "invalid_params" || !strings.Contains(structured["message"].(string), "confirmation") {
		t.Fatalf("tool error = %#v, want confirmation invalid_params", structured)
	}
	server.AssertNoRequests(t)
}

func TestChabDestructiveMCPToolsRequireConfirmationBeforeNetwork(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_destructive")
	server := testutil.NewAPIServer(t, testutil.FailOnContact(t))
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	tools := responseResult(t, h.request("tools/list", map[string]any{}))["tools"].([]any)
	for _, name := range []string{"chab_projects_delete", "chab_webhooks_deliveries_replay", "chab_webhooks_replays_create"} {
		tool := toolByName(t, tools, name)
		annotations := tool["annotations"].(map[string]any)
		if annotations["readOnlyHint"] != false || annotations["destructiveHint"] != true {
			t.Fatalf("%s annotations = %#v, want write/destructive", name, annotations)
		}
		input := tool["inputSchema"].(map[string]any)
		local := input["properties"].(map[string]any)["local"].(map[string]any)
		if !containsString(local["required"].([]any), "confirmation") {
			t.Fatalf("%s local schema = %#v, want required confirmation", name, local)
		}
	}
	resumeAnnotations := toolByName(t, tools, "chab_action_resume")["annotations"].(map[string]any)
	if resumeAnnotations["readOnlyHint"] != false || resumeAnnotations["destructiveHint"] != true {
		t.Fatalf("chab_action_resume annotations = %#v, want write/destructive", resumeAnnotations)
	}

	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{name: "chab_projects_delete", args: map[string]any{"path": map[string]any{"project_id": 42}}},
		{name: "chab_webhooks_deliveries_replay", args: map[string]any{"path": map[string]any{"delivery_id": "del_123"}}},
		{name: "chab_webhooks_replays_create", args: map[string]any{"body": map[string]any{
			"endpoint_id":    "whe_123",
			"event_types":    []string{"operation.succeeded"},
			"created_after":  "2026-09-13T00:00:00Z",
			"created_before": "2026-09-13T01:00:00Z",
		}}},
	} {
		result := responseResult(t, h.callTool(tc.name, tc.args))
		if result["isError"] != true {
			t.Fatalf("%s without confirmation should be tool error: %#v", tc.name, result)
		}
		structured := result["structuredContent"].(map[string]any)
		if structured["code"] != "invalid_params" || !strings.Contains(structured["message"].(string), "confirmation") {
			t.Fatalf("%s error = %#v, want confirmation invalid_params", tc.name, structured)
		}
	}
	server.AssertNoRequests(t)
}

func TestChabMCPFailedSubmissionKeepsRecoveryReceipt(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_partial")
	sourcePath := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(sourcePath, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/me":
			writeMCPWhoami(t, w)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/files":
			testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("internal_error", "temporary failure", "req-upload-fail", nil))
		default:
			t.Fatalf("unexpected MCP upload request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	result := responseResult(t, h.callTool("chab_files_create", map[string]any{
		"body":  map[string]any{"filename": "upload.txt"},
		"local": map[string]any{"confirmation": true, "input_path": sourcePath},
	}))
	if result["isError"] != true {
		t.Fatalf("failed upload should be a tool error: %#v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	if structured["action"] == nil {
		t.Fatalf("failed upload missing action receipt: %#v", structured)
	}
	recovery := structured["local_recovery"].(map[string]any)
	if recovery["can_resume"] != true || !strings.Contains(recovery["resume_hint"].(string), "chab_action_resume") {
		t.Fatalf("failed upload recovery = %#v, want resumable receipt", recovery)
	}
	errObj := structured["error"].(map[string]any)
	if errObj["code"] != "internal_error" || errObj["request_id"] != "req-upload-fail" {
		t.Fatalf("failed upload error = %#v", errObj)
	}
}

func TestChabMCPUploadMalformedResponseKeepsRecoveryReceipt(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_upload_malformed")
	sourcePath := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(sourcePath, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/me":
			writeMCPWhoami(t, w)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/files":
			testutil.WriteJSON(t, w, http.StatusAccepted, testutil.Envelope(map[string]any{}))
		default:
			t.Fatalf("unexpected MCP upload request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	result := responseResult(t, h.callTool("chab_files_create", map[string]any{
		"body":  map[string]any{"filename": "upload.txt"},
		"local": map[string]any{"confirmation": true, "input_path": sourcePath},
	}))
	if result["isError"] != true {
		t.Fatalf("malformed upload response should be a tool error: %#v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	if structured["action"] == nil || structured["server"] == nil {
		t.Fatalf("malformed upload response missing recovery receipt: %#v", structured)
	}
	recovery := structured["local_recovery"].(map[string]any)
	if recovery["can_resume"] != true || !strings.Contains(recovery["resume_hint"].(string), "chab_action_resume") {
		t.Fatalf("malformed upload recovery = %#v, want resumable receipt", recovery)
	}
	if structured["error"].(map[string]any)["code"] == "" {
		t.Fatalf("malformed upload error missing code: %#v", structured["error"])
	}
}

func TestChabMCPFileReadinessWaitHonorsTimeout(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_file_wait_timeout")
	sourcePath := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(sourcePath, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	var statusRequests atomic.Int32
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/me":
			writeMCPWhoami(t, w)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/files":
			testutil.WriteJSON(t, w, http.StatusAccepted, testutil.Envelope(map[string]any{
				"file": map[string]any{"id": "fil_wait", "state": "processing"},
			}))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/files/fil_wait":
			statusRequests.Add(1)
			<-r.Context().Done()
		default:
			t.Fatalf("unexpected MCP file status request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	result := responseResult(t, h.callTool("chab_files_create", map[string]any{
		"body":  map[string]any{"filename": "upload.txt"},
		"local": map[string]any{"confirmation": true, "input_path": sourcePath, "wait": true, "timeout_seconds": 1},
	}))
	if result["isError"] == true {
		t.Fatalf("file readiness timeout should return a recoverable receipt: %#v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	recovery := structured["local_recovery"].(map[string]any)
	if recovery["state"] != "timeout" || recovery["can_resume"] != true || recovery["known_remote"] != true {
		t.Fatalf("file readiness recovery = %#v, want timeout with remote receipt", recovery)
	}
	if statusRequests.Load() != 1 {
		t.Fatalf("file readiness status requests = %d, want 1", statusRequests.Load())
	}
}

func TestChabMCPFileDeleteWaitsOnFileLifecycle(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_file_delete")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/me":
			writeMCPWhoami(t, w)
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/v1/files/fil_delete":
			testutil.WriteJSON(t, w, http.StatusAccepted, testutil.Envelope(map[string]any{
				"file": map[string]any{"id": "fil_delete", "state": "deletion_pending"},
			}))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/files/fil_delete":
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{
				"file": map[string]any{"id": "fil_delete", "state": "deleted"},
			}))
		default:
			t.Fatalf("unexpected MCP file delete request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	result := responseResult(t, h.callTool("chab_files_delete", map[string]any{
		"path":  map[string]any{"file_id": "fil_delete"},
		"local": map[string]any{"confirmation": true, "wait": true},
	}))
	if result["isError"] == true {
		t.Fatalf("file delete returned error: %#v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	if structured["operation_id"] != "fil_delete" {
		t.Fatalf("file delete operation_id = %#v, want file id", structured["operation_id"])
	}
	recovery := structured["local_recovery"].(map[string]any)
	if recovery["can_resume"] != false || recovery["state"] != "completed" {
		t.Fatalf("file delete recovery = %#v, want completed", recovery)
	}
	action := structured["action"].(map[string]any)
	if action["state"] != "completed" || action["operation_key"] != "files.delete" {
		t.Fatalf("file delete action = %#v", action)
	}
}

func TestChabMCPMarkedFileDenialCannotResume(t *testing.T) {
	for _, kind := range []string{"upload", "delete"} {
		t.Run(kind, func(t *testing.T) {
			state := testutil.NewState(t)
			key := testutil.FakeKey("mcp_marked_file_" + kind)
			sourcePath := filepath.Join(t.TempDir(), "upload.txt")
			if err := os.WriteFile(sourcePath, []byte("payload"), 0o600); err != nil {
				t.Fatal(err)
			}
			starts := 0
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/me":
					writeMCPWhoami(t, w)
					return
				case r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/files" && kind == "upload":
					starts++
				case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/v1/files/fil_denied" && kind == "delete":
					starts++
				default:
					t.Fatalf("unexpected MCP file request %s %s", r.Method, r.URL.EscapedPath())
				}
				w.Header().Set("X-Chab-Idempotency-Outcome", "released")
				w.Header().Set("Retry-After", "1")
				testutil.WriteJSON(t, w, http.StatusTooManyRequests, testutil.ErrorEnvelope("rate_limited", "denied after claim", "req-file-denied", nil))
			})
			testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
			testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
			h := startChabHarness(t, map[string]string{"CHAB_CONFIG": state.ConfigPath, "CHAB_AUTH_FILE": state.AuthPath, "CHAB_PROFILE": "local"})
			tool := "chab_files_create"
			args := map[string]any{"body": map[string]any{"filename": "upload.txt"}, "local": map[string]any{"confirmation": true, "input_path": sourcePath}}
			if kind == "delete" {
				tool = "chab_files_delete"
				args = map[string]any{"path": map[string]any{"file_id": "fil_denied"}, "local": map[string]any{"confirmation": true}}
			}
			first := responseResult(t, h.callTool(tool, args))
			if first["isError"] != true || starts != 1 {
				t.Fatalf("first marked file denial = %#v, starts=%d", first, starts)
			}
			structured := first["structuredContent"].(map[string]any)
			if structured["action"].(map[string]any)["state"] != "denied" || structured["local_recovery"].(map[string]any)["can_resume"] != false {
				t.Fatalf("marked denial remained replayable: %#v", structured)
			}
			resumeArgs := map[string]any{"action_id": structured["action"].(map[string]any)["id"], "local": map[string]any{"confirmation": true, "input_path": sourcePath}}
			resumed := responseResult(t, h.callTool("chab_action_resume", resumeArgs))
			if resumed["isError"] != true || starts != 1 {
				t.Fatalf("denied file action replayed: %#v, starts=%d", resumed, starts)
			}
			if !strings.Contains(fmt.Sprint(resumed), "was denied by the API and cannot be resumed") {
				t.Fatalf("resume did not reach denied-action journal guard: %#v", resumed)
			}
		})
	}
}

func TestChabMCPUnknownFileReplaySettledErrorStaysUnknown(t *testing.T) {
	for _, kind := range []string{"upload", "delete"} {
		t.Run(kind, func(t *testing.T) {
			state := testutil.NewState(t)
			key := testutil.FakeKey("mcp_unknown_file_" + kind)
			sourcePath := filepath.Join(t.TempDir(), "upload.txt")
			if err := os.WriteFile(sourcePath, []byte("payload"), 0o600); err != nil {
				t.Fatal(err)
			}
			starts := 0
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/me":
					writeMCPWhoami(t, w)
					return
				case r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/files" && kind == "upload":
					starts++
				case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/v1/files/fil_unknown" && kind == "delete":
					starts++
				default:
					t.Fatalf("unexpected MCP file request %s %s", r.Method, r.URL.EscapedPath())
				}
				if starts == 1 {
					testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("server_error", "uncertain", "req-first", nil))
				} else {
					w.Header().Set("X-Chab-Idempotency-Outcome", "settled")
					testutil.WriteJSON(t, w, http.StatusServiceUnavailable, testutil.ErrorEnvelope("provider_unavailable", "saved response", "req-replay", nil))
				}
			})
			testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
			testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
			h := startChabHarness(t, map[string]string{"CHAB_CONFIG": state.ConfigPath, "CHAB_AUTH_FILE": state.AuthPath, "CHAB_PROFILE": "local"})
			tool := "chab_files_create"
			args := map[string]any{"body": map[string]any{"filename": "upload.txt"}, "local": map[string]any{"confirmation": true, "input_path": sourcePath}}
			if kind == "delete" {
				tool = "chab_files_delete"
				args = map[string]any{"path": map[string]any{"file_id": "fil_unknown"}, "local": map[string]any{"confirmation": true}}
			}
			first := responseResult(t, h.callTool(tool, args))
			if first["isError"] != true {
				t.Fatalf("first outcome should be uncertain: %#v", first)
			}
			action := first["structuredContent"].(map[string]any)["action"].(map[string]any)
			resumeLocal := map[string]any{"confirmation": true}
			if kind == "upload" {
				resumeLocal["input_path"] = sourcePath
			}
			resume := responseResult(t, h.callTool("chab_action_resume", map[string]any{"action_id": action["id"], "local": resumeLocal}))
			structured := resume["structuredContent"].(map[string]any)
			if resume["isError"] != true || starts != 2 || structured["action"].(map[string]any)["state"] != "unknown" || structured["local_recovery"].(map[string]any)["can_resume"] != true {
				t.Fatalf("settled replay lost unknown file receipt: %#v, starts=%d", resume, starts)
			}
		})
	}
}

func TestChabMCPActionResumeSupportsBodylessRequests(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_bodyless_resume")
	deleteAttempts := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/me":
			writeMCPWhoami(t, w)
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/v1/projects/42":
			deleteAttempts++
			if deleteAttempts == 1 {
				testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("internal_error", "unknown outcome", "req-delete-fail", nil))
				return
			}
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{
				"project": map[string]any{"id": 42, "deleted": true},
			}))
		default:
			t.Fatalf("unexpected MCP project delete request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	first := responseResult(t, h.callTool("chab_projects_delete", map[string]any{
		"path":  map[string]any{"project_id": 42},
		"local": map[string]any{"confirmation": true},
	}))
	if first["isError"] != true {
		t.Fatalf("first delete should be unknown-outcome error: %#v", first)
	}
	actionID := first["structuredContent"].(map[string]any)["action"].(map[string]any)["id"].(string)

	resumed := responseResult(t, h.callTool("chab_action_resume", map[string]any{
		"action_id": actionID,
		"local":     map[string]any{"confirmation": true},
	}))
	if resumed["isError"] == true {
		t.Fatalf("bodyless resume returned error: %#v", resumed)
	}
	if deleteAttempts != 2 {
		t.Fatalf("delete attempts = %d, want 2", deleteAttempts)
	}
	structured := resumed["structuredContent"].(map[string]any)
	if structured["local_recovery"].(map[string]any)["can_resume"] != false {
		t.Fatalf("bodyless resume recovery = %#v", structured["local_recovery"])
	}
}

func TestChabMCPActionResumeFailureKeepsRecoveryReceipt(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_resume_partial")
	deleteAttempts := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/me":
			writeMCPWhoami(t, w)
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/v1/projects/42":
			deleteAttempts++
			testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("internal_error", "unknown outcome", "req-delete-fail", nil))
		default:
			t.Fatalf("unexpected MCP project delete request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	first := responseResult(t, h.callTool("chab_projects_delete", map[string]any{
		"path":  map[string]any{"project_id": 42},
		"local": map[string]any{"confirmation": true},
	}))
	if first["isError"] != true {
		t.Fatalf("first delete should be unknown-outcome error: %#v", first)
	}
	actionID := first["structuredContent"].(map[string]any)["action"].(map[string]any)["id"].(string)

	resumed := responseResult(t, h.callTool("chab_action_resume", map[string]any{
		"action_id": actionID,
		"local":     map[string]any{"confirmation": true},
	}))
	if resumed["isError"] != true {
		t.Fatalf("failed resume should be a tool error: %#v", resumed)
	}
	structured := resumed["structuredContent"].(map[string]any)
	if structured["action"] == nil {
		t.Fatalf("failed resume missing action receipt: %#v", structured)
	}
	recovery := structured["local_recovery"].(map[string]any)
	if recovery["can_resume"] != true || recovery["resume_hint"].(string) == "" {
		t.Fatalf("failed resume recovery = %#v, want resumable receipt", recovery)
	}
	errObj := structured["error"].(map[string]any)
	if errObj["code"] != "internal_error" || errObj["request_id"] != "req-delete-fail" {
		t.Fatalf("failed resume error = %#v", errObj)
	}
	if deleteAttempts != 2 {
		t.Fatalf("delete attempts = %d, want 2", deleteAttempts)
	}
}

func TestChabMCPActionResumeKnownOperationDoesNotWaitUnlessRequested(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_resume_known")
	statusRequests := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/me":
			writeMCPWhoami(t, w)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/screenshots/url":
			testutil.WriteJSON(t, w, http.StatusAccepted, testutil.Envelope(map[string]any{
				"id":               "op_resume",
				"operation_key":    "screenshots.url",
				"family":           "screenshots",
				"status":           "queued",
				"result_available": false,
			}))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/operations/op_resume":
			statusRequests++
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{
				"id":               "op_resume",
				"operation_key":    "screenshots.url",
				"family":           "screenshots",
				"status":           "succeeded",
				"result_available": true,
			}))
		default:
			t.Fatalf("unexpected MCP operation request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	first := responseResult(t, h.callTool("chab_screenshots_url", map[string]any{
		"body":  map[string]any{"url": "https://example.com/"},
		"local": map[string]any{"confirmation": true},
	}))
	if first["isError"] == true {
		t.Fatalf("screenshot start returned error: %#v", first)
	}
	actionID := first["structuredContent"].(map[string]any)["action"].(map[string]any)["id"].(string)
	resumed := responseResult(t, h.callTool("chab_action_resume", map[string]any{
		"action_id": actionID,
	}))
	if resumed["isError"] == true {
		t.Fatalf("known operation resume returned error: %#v", resumed)
	}
	if statusRequests != 0 {
		t.Fatalf("known operation resume status requests = %d, want 0 without local.wait", statusRequests)
	}
}

func TestChabMCPActionResumeUploadHonorsWait(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_resume_upload_wait")
	sourcePath := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(sourcePath, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	uploadAttempts := 0
	statusRequests := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/me":
			writeMCPWhoami(t, w)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/files":
			uploadAttempts++
			if uploadAttempts == 1 {
				testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("internal_error", "temporary failure", "req-upload-fail", nil))
				return
			}
			testutil.WriteJSON(t, w, http.StatusAccepted, testutil.Envelope(map[string]any{
				"file": map[string]any{"id": "fil_resume_wait", "state": "processing"},
			}))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/files/fil_resume_wait":
			statusRequests++
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{
				"file": map[string]any{"id": "fil_resume_wait", "state": "available"},
			}))
		default:
			t.Fatalf("unexpected MCP upload resume request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})

	first := responseResult(t, h.callTool("chab_files_create", map[string]any{
		"body":  map[string]any{"filename": "upload.txt"},
		"local": map[string]any{"confirmation": true, "input_path": sourcePath},
	}))
	if first["isError"] != true {
		t.Fatalf("first upload should be unknown-outcome error: %#v", first)
	}
	actionID := first["structuredContent"].(map[string]any)["action"].(map[string]any)["id"].(string)
	resumed := responseResult(t, h.callTool("chab_action_resume", map[string]any{
		"action_id": actionID,
		"body":      map[string]any{"filename": "upload.txt"},
		"local":     map[string]any{"confirmation": true, "input_path": sourcePath, "wait": true},
	}))
	if resumed["isError"] == true {
		t.Fatalf("upload resume returned error: %#v", resumed)
	}
	if uploadAttempts != 2 {
		t.Fatalf("upload attempts = %d, want 2", uploadAttempts)
	}
	if statusRequests != 1 {
		t.Fatalf("upload resume status requests = %d, want 1", statusRequests)
	}
	recovery := resumed["structuredContent"].(map[string]any)["local_recovery"].(map[string]any)
	if recovery["can_resume"] != false {
		t.Fatalf("upload resume recovery = %#v, want completed", recovery)
	}
}

func TestChabMCPActionResumeSendsApprovalProof(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_proof_resume")
	proofPath := filepath.Join(t.TempDir(), "approval.json")
	if err := os.WriteFile(proofPath, []byte(`{"proof":"synthetic-approval-proof"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tokenAttempts := 0
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/me":
			writeMCPWhoami(t, w)
		case r.Method == http.MethodPatch && r.URL.EscapedPath() == "/v1/tokens/ak_target_public":
			tokenAttempts++
			if got := r.Header.Get("X-Chab-Management-Approval"); got != "synthetic-approval-proof" {
				t.Fatalf("request %d missing management approval header", tokenAttempts)
			}
			if tokenAttempts == 1 {
				testutil.WriteJSON(t, w, http.StatusInternalServerError, testutil.ErrorEnvelope("internal_error", "unknown outcome", "req-token-fail", nil))
				return
			}
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{
				"token": map[string]any{"token_public_id": "ak_target_public", "policy_revision": 4, "disabled": true},
			}))
		default:
			t.Fatalf("unexpected MCP token update request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})
	body := map[string]any{"expected_policy_revision": 3, "disabled": true}

	first := responseResult(t, h.callTool("chab_tokens_update", map[string]any{
		"path":  map[string]any{"token_public_id": "ak_target_public"},
		"body":  body,
		"local": map[string]any{"confirmation": true, "approval_proof_path": proofPath},
	}))
	if first["isError"] != true {
		t.Fatalf("first token update should be unknown-outcome error: %#v", first)
	}
	actionID := first["structuredContent"].(map[string]any)["action"].(map[string]any)["id"].(string)

	resumed := responseResult(t, h.callTool("chab_action_resume", map[string]any{
		"action_id": actionID,
		"body":      body,
		"local":     map[string]any{"confirmation": true, "approval_proof_path": proofPath},
	}))
	if resumed["isError"] == true {
		t.Fatalf("proof-backed resume returned error: %#v", resumed)
	}
	if tokenAttempts != 2 {
		t.Fatalf("token update attempts = %d, want 2", tokenAttempts)
	}
}

func TestChabMCPBodySchemasBundleReferencedComponents(t *testing.T) {
	h := startChabHarness(t, nil)
	tools := responseResult(t, h.request("tools/list", map[string]any{}))["tools"].([]any)
	for _, name := range []string{"chab_llm_generate", "chab_scrape_dom"} {
		tool := toolByName(t, tools, name)
		inputSchema := tool["inputSchema"].(map[string]any)
		assertSchemaRefsResolvable(t, name+" input", inputSchema)
	}
}

func TestChabMCPDryRunOutputSchemaIncludesLiveAndDryRunShapes(t *testing.T) {
	h := startChabHarness(t, nil)
	tools := responseResult(t, h.request("tools/list", map[string]any{}))["tools"].([]any)
	research := toolByName(t, tools, "chab_research_deep")
	assertSchemaRefsResolvable(t, "chab_research_deep output", research["outputSchema"].(map[string]any))

	screenshot := toolByName(t, tools, "chab_screenshots_url")
	outputSchema := screenshot["outputSchema"].(map[string]any)
	assertSchemaRefsResolvable(t, "chab_screenshots_url output", outputSchema)
	topVariants := outputSchema["oneOf"].([]any)
	var operationVariants []any
	for _, variant := range topVariants {
		if schema, ok := variant.(map[string]any); ok {
			if nested, ok := schema["oneOf"].([]any); ok {
				operationVariants = nested
			}
		}
	}
	if len(operationVariants) == 0 {
		t.Fatalf("screenshot output schema = %#v, want operation oneOf variants", outputSchema)
	}
	var hasActionOutput, hasDryRunOutput bool
	for _, variant := range operationVariants {
		schema := variant.(map[string]any)
		props := schema["properties"].(map[string]any)
		if props["local_recovery"] != nil {
			hasActionOutput = true
		}
		if props["operation"] != nil && props["server"] != nil {
			serverSchema := props["server"].(map[string]any)
			required := requiredStrings(serverSchema)
			if !containsStringString(required, "estimated_credits") || !containsStringString(required, "live_request") {
				t.Fatalf("dry-run server required = %#v, want dry-run preview fields", required)
			}
			for _, finalField := range []string{"dimensions", "mime_type", "captured_at"} {
				if containsStringString(required, finalField) {
					t.Fatalf("dry-run server required = %#v, contains final result field %s", required, finalField)
				}
			}
			hasDryRunOutput = true
		}
	}
	if !hasActionOutput || !hasDryRunOutput {
		t.Fatalf("screenshot operation variants = %#v, want action and dry-run outputs", operationVariants)
	}
}

func TestChabByteDownloadRequiresExplicitOutputAndNoOverwrite(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_download")
	bytesOut := []byte("file-bytes")
	sum := sha256.Sum256(bytesOut)
	checksum := "sha256:" + hex.EncodeToString(sum[:])
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/files/fil_123":
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{
				"file": map[string]any{
					"id":           "fil_123",
					"state":        "available",
					"download_url": "/v1/files/fil_123/download",
					"checksum":     checksum,
				},
			}))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/files/fil_123/download":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(bytesOut)
		default:
			t.Fatalf("unexpected MCP download request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})
	outPath := filepath.Join(t.TempDir(), "download.bin")
	result := responseResult(t, h.callTool("chab_files_download", map[string]any{
		"path":  map[string]any{"file_id": "fil_123"},
		"local": map[string]any{"output_path": outPath, "max_bytes": 1024},
	}))
	if result["isError"] == true {
		t.Fatalf("download returned error: %#v", result)
	}
	if got := result["structuredContent"].(map[string]any)["checksum"]; got != checksum {
		t.Fatalf("download checksum = %#v, want %s", got, checksum)
	}
	if data, err := os.ReadFile(outPath); err != nil || string(data) != "file-bytes" {
		t.Fatalf("downloaded file = %q, %v", data, err)
	}
	again := responseResult(t, h.callTool("chab_files_download", map[string]any{
		"path":  map[string]any{"file_id": "fil_123"},
		"local": map[string]any{"output_path": outPath, "max_bytes": 1024},
	}))
	if again["isError"] != true {
		t.Fatalf("overwrite should be refused: %#v", again)
	}
}

func TestChabByteDownloadRejectsChecksumMismatch(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("mcp_download_mismatch")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/operations/op_123/artifacts/art_123":
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{
				"artifact": map[string]any{
					"id":           "art_123",
					"download_url": "/v1/operations/op_123/artifacts/art_123/download",
					"checksum":     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				},
			}))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/operations/op_123/artifacts/art_123/download":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("wrong-bytes"))
		default:
			t.Fatalf("unexpected MCP artifact download request %s %s", r.Method, r.URL.EscapedPath())
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	h := startChabHarness(t, map[string]string{
		"CHAB_CONFIG":    state.ConfigPath,
		"CHAB_AUTH_FILE": state.AuthPath,
		"CHAB_PROFILE":   "local",
	})
	outPath := filepath.Join(t.TempDir(), "artifact.bin")
	result := responseResult(t, h.callTool("chab_operations_artifact_download", map[string]any{
		"path":  map[string]any{"id": "op_123", "artifact_id": "art_123"},
		"local": map[string]any{"output_path": outPath, "max_bytes": 1024},
	}))
	if result["isError"] != true {
		t.Fatalf("checksum mismatch should be refused: %#v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	if structured["code"] != "invalid_params" || !strings.Contains(structured["message"].(string), "checksum mismatch") {
		t.Fatalf("checksum mismatch error = %#v", structured)
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatalf("mismatched download published output file: %v", err)
	}
	server.AssertBearer(t, key)
}

type chabHarness struct {
	t      *testing.T
	in     *io.PipeWriter
	out    *bufio.Reader
	cancel context.CancelFunc
	errCh  chan error
	nextID int
	stderr *bytes.Buffer
}

func containsString(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsStringString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertMCPResultNotContains(t *testing.T, result map[string]any, forbidden ...string) {
	t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range forbidden {
		if value != "" && strings.Contains(string(encoded), value) {
			t.Fatalf("MCP result leaked a registered secret: %s", encoded)
		}
	}
}

func requiredStrings(schema map[string]any) []string {
	values, _ := schema["required"].([]any)
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func assertSchemaRefsResolvable(t *testing.T, label string, schema map[string]any) {
	t.Helper()
	components, _ := schema["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	var walk func(path string, value any)
	walk = func(path string, value any) {
		switch typed := value.(type) {
		case map[string]any:
			if ref, _ := typed["$ref"].(string); ref != "" {
				const prefix = "#/components/schemas/"
				if !strings.HasPrefix(ref, prefix) {
					t.Fatalf("%s %s uses unsupported ref %q", label, path, ref)
				}
				name := strings.TrimPrefix(ref, prefix)
				if schemas[name] == nil {
					t.Fatalf("%s %s has unresolved ref %q", label, path, ref)
				}
			}
			for key, child := range typed {
				walk(path+"."+key, child)
			}
		case []any:
			for index, child := range typed {
				walk(path+"[]"+string(rune('0'+index)), child)
			}
		}
	}
	walk("$", schema)
}

func toolByName(t *testing.T, tools []any, name string) map[string]any {
	t.Helper()
	for _, tool := range tools {
		row := tool.(map[string]any)
		if row["name"] == name {
			return row
		}
	}
	t.Fatalf("tool %s not found", name)
	return nil
}

func writeMCPWhoami(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{
		"principal_type":  "team_api_token",
		"team_id":         42,
		"token_id":        "tok_internal",
		"token_public_id": "870b679b-210c-4d33-9d2c-a5b091693f42",
		"scopes":          []string{"api:*"},
		"token_controls": map[string]any{
			"policy_revision": 1,
			"feature_access":  map[string]any{"mode": "all", "snapshot_stale": false},
			"ip_restrictions": map[string]any{"restricted": false, "allow_rule_count": 0, "deny_rule_count": 0},
			"spending":        map[string]any{"mode": "enabled", "active_reserved_credits": 0, "allowance": nil},
			"project_access":  map[string]any{"mode": "all", "selected_project_ids": []int{}, "selected_count": 0},
		},
		"request_id": "req-whoami",
	}))
}

func startChabHarness(t *testing.T, env map[string]string) *chabHarness {
	t.Helper()
	if env == nil {
		state := testutil.NewState(t)
		env = map[string]string{
			"CHAB_CONFIG":    state.ConfigPath,
			"CHAB_AUTH_FILE": state.AuthPath,
		}
	}
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	var stderr bytes.Buffer
	root := &cobra.Command{Use: "chab"}
	cmd := &cobra.Command{Use: "serve"}
	root.PersistentFlags().Bool("debug", false, "")
	root.PersistentFlags().Bool("json", false, "")
	root.PersistentFlags().Bool("include-meta", false, "")
	root.PersistentFlags().Bool("plain", false, "")
	root.PersistentFlags().String("jq", "", "")
	root.PersistentFlags().String("template", "", "")
	root.AddCommand(cmd)
	cmd.SetErr(&stderr)
	cmd.SetOut(stdoutW)
	cmd.SetIn(stdinR)

	factory := &cmdutil.Factory{
		LookupEnv: testutil.HermeticEnv(env),
		Version:   "test-version",
		Secrets:   redact.NewRegistry(),
		Sleeper:   noSleep{},
		Now:       func() time.Time { return time.Unix(0, 0).UTC() },
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- mcpserver.Serve(ctx, mcpserver.Options{
			Factory: factory,
			Command: cmd,
			Version: "test-version",
			Stdin:   stdinR,
			Stdout:  stdoutW,
			Stderr:  &stderr,
		})
	}()
	h := &chabHarness{t: t, in: stdinW, out: bufio.NewReader(stdoutR), cancel: cancel, errCh: errCh, stderr: &stderr}
	t.Cleanup(func() {
		cancel()
		_ = stdinW.Close()
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		select {
		case <-errCh:
		case <-time.After(2 * time.Second):
			t.Fatalf("MCP server did not stop; stderr=%s", stderr.String())
		}
	})
	return h
}

func (h *chabHarness) request(method string, params map[string]any) map[string]any {
	h.t.Helper()
	h.nextID++
	if params == nil {
		params = map[string]any{}
	}
	params["_meta"] = map[string]any{
		"io.modelcontextprotocol/protocolVersion":    mcpserver.ProtocolVersion,
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "mcpserver-test", "version": "1.0.0"},
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      h.nextID,
		"method":  method,
		"params":  params,
	}
	line, err := json.Marshal(payload)
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.in.Write(append(line, '\n')); err != nil {
		h.t.Fatalf("write request: %v", err)
	}
	resp, err := h.out.ReadBytes('\n')
	if err != nil {
		h.t.Fatalf("read response: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(resp, &decoded); err != nil {
		h.t.Fatalf("decode response %s: %v", resp, err)
	}
	return decoded
}

func (h *chabHarness) callTool(name string, args map[string]any) map[string]any {
	h.t.Helper()
	return h.request("tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
}

func responseResult(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	if errObj, ok := resp["error"]; ok {
		t.Fatalf("response error = %#v", errObj)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("response missing object result: %#v", resp)
	}
	return result
}

type noSleep struct{}

func (noSleep) Sleep(ctx context.Context, _ time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func TestChabStdoutFramesAreProtocolOnly(t *testing.T) {
	h := startChabHarness(t, nil)
	resp := h.request("server/discover", map[string]any{})
	if responseResult(t, resp)["resultType"] != "complete" {
		t.Fatalf("discover response = %#v", resp)
	}
	if stderr := h.stderr.String(); stderr != "" && strings.Contains(stderr, "Usage:") {
		t.Fatalf("stderr contains human usage: %s", stderr)
	}
}
