package cli_test

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/chab-cli/internal/auth"
)

var commandNow = time.Date(2026, 7, 14, 13, 35, 42, 987654321, time.FixedZone("CEST", 2*60*60))

func requireWhoamiRequest(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodGet || r.URL.Path != "/v1/me" {
		t.Fatalf("request = %s %s, want GET /v1/me", r.Method, r.URL.Path)
	}
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	retryable := code == "rate_limited" || code == "internal_error" || code == "temporarily_unavailable"
	fmt.Fprintf(w, `{"error":{"code":%q,"message":%q,"retryable":%t,"details":null}}`, code, message, retryable)
}

func whoamiEnvelope() string {
	return `{
  "data": {
    "principal_type": "team",
    "team_id": 42,
    "token_id": "tok_internal_01HY0000000000000000000000",
    "token_public_id": "ak_01HY0000000000000000000000",
    "scopes": ["api:projects:read", "api:credits:read"],
    "token_controls": {
      "policy_revision": 7,
      "feature_access": {"mode": "full", "snapshot_stale": false},
      "ip_restrictions": {"restricted": false, "allow_rule_count": 0, "deny_rule_count": 0},
      "spending": {
        "mode": "enabled",
        "active_reserved_credits": 0,
        "allowance": {
          "version": 1,
          "limit_credits": 1000,
          "settled_credits": 100,
          "held_credits": 0,
          "remaining_credits": 900
        }
      },
      "project_access": {"mode": "all", "selected_project_ids": [1, 2], "selected_count": 2}
    }
  },
  "request_id": "req-whoami"
}`
}

func compatibilityEnvelope() string {
	return `{
  "data": {
    "schema_version": "1.0.0",
    "api_major": 1,
    "minimum_version": "1.0.0",
    "recommended_version": "1.0.0",
    "catalog_version": "2026-09-12",
    "blocked_ranges": [],
    "notices": [],
    "urls": {}
  },
  "request_id": "req-compat"
}`
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s exists or stat failed unexpectedly: %v", path, err)
	}
}

func assertStoredAuthKey(t *testing.T, path, profile, want string) {
	t.Helper()
	file, _, err := auth.Load(path)
	if err != nil {
		t.Fatalf("auth.Load(%q) error = %v", path, err)
	}
	record, ok := file.Profiles[profile]
	if !ok {
		t.Fatalf("profile %q missing from auth file", profile)
	}
	if record.APIKey != want {
		t.Fatalf("stored key for %q was not the expected key", profile)
	}
}

func assertStoredChabToken(t *testing.T, path, profile, wantKey string) {
	t.Helper()
	file, _, err := auth.Load(path)
	if err != nil {
		t.Fatalf("auth.Load(%q) error = %v", path, err)
	}
	record, ok := file.Profiles[profile]
	if !ok {
		t.Fatalf("profile %q missing from auth file", profile)
	}
	if record.APIKey != wantKey ||
		record.PrincipalType != "team" ||
		record.TeamID != 42 ||
		record.TokenPublicID != "ak_01HY0000000000000000000000" ||
		record.TokenID != "tok_internal_01HY0000000000000000000000" ||
		record.TokenControls == nil ||
		record.TokenControls.PolicyRevision != 7 ||
		record.LastValidatedAt == nil ||
		record.LastValidatedAt.Format(time.RFC3339) != "2026-07-14T11:35:42Z" {
		t.Fatalf("stored auth record = %#v", record)
	}
	if got := strings.Join(record.Scopes, " "); got != "api:projects:read api:credits:read" {
		t.Fatalf("stored scopes = %q", got)
	}
}

type findingSummary struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
}

func hasFinding(findings []findingSummary, id, severity string) bool {
	for _, finding := range findings {
		if finding.ID == id && (severity == "" || finding.Severity == severity) {
			return true
		}
	}
	return false
}
