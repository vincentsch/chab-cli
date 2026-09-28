package cli_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestHealthPreservesFreeModeAndCoveredOperationKeys(t *testing.T) {
	state := testutil.NewState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/v1/health" {
			t.Fatalf("unexpected health request %s %s", r.Method, r.URL.EscapedPath())
		}
		testutil.WriteJSON(t, w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"service": "chab", "api_version": "v1", "status": "ok",
				"updated_at": "2026-09-24T17:00:00Z",
				"families": []map[string]any{
					{"family": "search", "state": "healthy", "covered_operation_keys": []string{"search.web"}, "remediation": nil},
					{"family": "llm", "state": "unknown", "covered_operation_keys": nil, "remediation": "Provider not ready"},
				},
				"free_mode": map[string]any{"state": "enabled"},
			},
			"request_id": "req-health-coverage",
		})
	})

	result := testutil.RunCommand(t, state.APIArgs(server, "health", "--json")...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("health JSON result = %#v", result)
	}
	var body struct {
		FreeMode struct {
			State string `json:"state"`
		} `json:"free_mode"`
		Families []struct {
			CoveredOperationKeys *[]string `json:"covered_operation_keys"`
		} `json:"families"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &body); err != nil {
		t.Fatalf("decode health JSON: %v: %s", err, result.Stdout)
	}
	if body.FreeMode.State != "enabled" || len(body.Families) != 2 ||
		body.Families[0].CoveredOperationKeys == nil ||
		!reflect.DeepEqual(*body.Families[0].CoveredOperationKeys, []string{"search.web"}) ||
		body.Families[1].CoveredOperationKeys != nil {
		t.Fatalf("health coverage lost: %#v", body)
	}

	plain := testutil.RunCommand(t, state.APIArgs(server, "health", "--plain")...)
	if plain.ExitCode != cli.ExitSuccess || plain.Err != nil || plain.Stderr != "" ||
		!strings.Contains(plain.Stdout, "free_mode\tenabled") ||
		!strings.Contains(plain.Stdout, "search=healthy [search.web]") {
		t.Fatalf("health plain result = %#v", plain)
	}
	for i := range server.Requests() {
		if got := server.Request(t, i).Header.Get("Authorization"); got != "" {
			t.Fatalf("public health request %d carried Authorization: %q", i, got)
		}
	}
}
