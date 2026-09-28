package mcpserver_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/chabcontract"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/testutil"
	"gopkg.in/yaml.v3"
)

func TestFullBetaGuestMCPVisibilityConfirmationAndRevocation(t *testing.T) {
	keys := []string{"search.web", "search.serp", "seo.keywords.ideas", "seo.keywords.metrics", "seo.domains.overview", "seo.domains.backlinks", "business.search", "business.details", "contacts.domain_search", "contacts.email_finder", "contacts.email_verify", "scrape.markdown", "scrape.dom", "screenshots.url", "convert.file", "llm.generate", "llm.embeddings", "translate.text_or_document", "research.deep"}
	registry := chabcontract.MustLoad()
	helpers := []string{"auth.me", "credits.get", "operations.get", "operations.result", "operations.cancel", "files.list", "files.get", "files.delete", "files.download", "llm.models"}
	allScopes := []string{"api:credits:read", "api:files:read", "api:files:write", "api:llm:read"}
	for _, key := range keys {
		op, ok := registry.Find(key)
		if !ok || !op.IdempotencyRequired || !op.RequiresPaidPlan || op.NotBillable {
			t.Fatalf("missing effectful pinned beta operation %s", key)
		}
		if !slices.Contains(allScopes, op.RequiredScope) {
			allScopes = append(allScopes, op.RequiredScope)
		}
	}
	var scopes, removedRoute atomic.Value
	scopes.Store(allScopes)
	removedRoute.Store("")
	var paused, malformed atomic.Bool
	var effectCalls atomic.Int32
	state := testutil.NewState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/v1/me":
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{"principal_type": "guest_trial", "principal_id": "guest_trial:full_beta", "guest_id": "full_beta", "scopes": scopes.Load().([]string)}))
		case "/v1/cli/compatibility":
			routes := append([]string{}, helpers...)
			tools := []map[string]any{}
			advertised := []string{}
			if !paused.Load() {
				routes = append(routes, "files.create")
				for _, key := range keys {
					if key == removedRoute.Load().(string) {
						continue
					}
					op, _ := registry.Find(key)
					path := op.Path
					if malformed.Load() && key == "convert.file" {
						path = "/v1/tokens"
					}
					routes = append(routes, key)
					advertised = append(advertised, key)
					tools = append(tools, map[string]any{"tool_name": betaToolName(key), "operation_key": key, "scope": op.RequiredScope, "method": op.Method, "path": path, "idempotency_required": true, "funding_mode": "promotional_only", "credit_source": "guest_trial"})
				}
			}
			testutil.WriteJSON(t, w, http.StatusOK, testutil.Envelope(map[string]any{"schema_version": "1.0.0", "api_major": 1, "minimum_version": "0.1.0", "recommended_version": "0.1.0", "catalog_version": "full-beta-test", "local_mcp": map[string]any{"transport": "stdio", "api_transport": "rest", "guest_trial_credentials": map[string]any{"accepted": true, "enabled": true, "spend_enabled": !paused.Load(), "supported_route_operation_keys": routes, "operation_keys": advertised, "tools": tools}}}))
		default:
			effectCalls.Add(1)
			t.Errorf("unconfirmed or revoked guest tool contacted operation endpoint %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusForbidden)
		}
	})
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{APIBaseURL: server.APIBaseURL(), Locale: "en"}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", auth.ProfileAuth{APIKey: "chab_guest_full_beta_dummy", PrincipalType: "guest_trial", PrincipalID: "guest_trial:full_beta"})
	h := startChabHarness(t, map[string]string{"CHAB_CONFIG": state.ConfigPath, "CHAB_AUTH_FILE": state.AuthPath, "CHAB_PROFILE": "local"})
	names := func() []string {
		listed := responseResult(t, h.request("tools/list", map[string]any{}))["tools"].([]any)
		result := []string{}
		for _, raw := range listed {
			result = append(result, raw.(map[string]any)["name"].(string))
		}
		return result
	}
	listed := names()
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			if !slices.Contains(listed, betaToolName(key)) {
				t.Fatalf("full beta does not advertise %s", key)
			}
			result := responseResult(t, h.callTool(betaToolName(key), map[string]any{"body": betaPinnedExample(t, key)}))
			if result["isError"] != true || !strings.Contains(result["structuredContent"].(map[string]any)["message"].(string), "confirmation") {
				t.Fatalf("%s must require confirmation before effect: %#v", key, result)
			}
		})
	}
	assertDenied := func(key string) {
		t.Helper()
		if slices.Contains(names(), betaToolName(key)) {
			t.Fatalf("removed authority still advertises %s", key)
		}
		args := map[string]any{"local": map[string]any{"confirmation": true}}
		if key == "files.create" {
			path := filepath.Join(t.TempDir(), "owned-guest-upload.txt")
			if err := os.WriteFile(path, []byte("owned denied guest upload fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			args["body"] = map[string]any{"filename": "owned-guest-upload.txt"}
			args["local"] = map[string]any{"confirmation": true, "input_path": path}
		} else {
			args["body"] = betaPinnedExample(t, key)
		}
		result := responseResult(t, h.callTool(betaToolName(key), args))
		message := result["structuredContent"].(map[string]any)["message"].(string)
		if result["isError"] != true || (!strings.Contains(message, "not available") && !(malformed.Load() && strings.Contains(message, "compatibility"))) {
			t.Fatalf("removed authority still calls %s: %#v", key, result)
		}
	}
	research, _ := registry.Find("research.deep")
	limited := slices.DeleteFunc(append([]string{}, allScopes...), func(scope string) bool { return scope == research.RequiredScope })
	scopes.Store(limited)
	assertDenied("research.deep")
	scopes.Store(allScopes)
	removedRoute.Store("convert.file")
	assertDenied("convert.file")
	removedRoute.Store("")
	paused.Store(true)
	for _, key := range keys {
		assertDenied(key)
	}
	for _, key := range []string{"files.list", "files.get", "files.delete", "files.download", "llm.models"} {
		if !slices.Contains(names(), betaToolName(key)) {
			t.Fatalf("pause hid advertised nonspending helper %s", key)
		}
	}
	assertDenied("files.create")
	paused.Store(false)
	malformed.Store(true)
	for _, key := range keys {
		assertDenied(key)
	}
	if effectCalls.Load() != 0 {
		t.Fatalf("guest confirmation/authority fences sent %d effect requests", effectCalls.Load())
	}
}

func betaToolName(key string) string { return "chab_" + strings.ReplaceAll(key, ".", "_") }

// Valid request examples come from the same pinned OpenAPI contract as the
// operation metadata. No provider or customer file is involved in these calls.
func betaPinnedExample(t *testing.T, key string) any {
	t.Helper()
	data, err := os.ReadFile("../chabcontract/testdata/chab-v1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]map[string]struct {
			OperationID string `yaml:"operationId"`
			RequestBody struct {
				Content map[string]struct {
					Examples map[string]struct {
						Value any `yaml:"value"`
					} `yaml:"examples"`
				} `yaml:"content"`
			} `yaml:"requestBody"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	for _, methods := range document.Paths {
		for _, operation := range methods {
			if operation.OperationID != key {
				continue
			}
			for _, example := range operation.RequestBody.Content["application/json"].Examples {
				body, ok := example.Value.(map[string]any)
				if !ok {
					continue
				}
				delete(body, "dry_run")
				delete(body, "callback")
				delete(body, "callback_url")
				delete(body, "callback_secret")
				encoded, err := json.Marshal(body)
				if err == nil && chabcontract.MustLoad().ValidateRequest(key, encoded) == nil {
					return body
				}
			}
		}
	}
	t.Fatalf("no valid pinned example for %s", key)
	return nil
}
