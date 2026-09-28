package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestManualLoginStoresChabTokenMetadata(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("login_chab")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		requireWhoamiRequest(t, r)
		fmt.Fprint(w, whoamiEnvelope())
	})

	result := testutil.RunCommandWith(t, testutil.Options{
		Stdin: strings.NewReader(key + "\n"),
		Now:   func() time.Time { return commandNow },
	}, state.APIArgs(server, "login", "--api-key", "--no-prompt", "--plain")...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("login result = %#v", result)
	}
	for _, want := range []string{
		"profile\tlocal",
		"team_id\t42",
		"token_public_id\tak_01HY0000000000000000000000",
		"principal_type\tteam",
		"scopes\tapi:projects:read api:credits:read",
		"stored\ttrue",
		"authenticated\ttrue",
	} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("login plain missing %q:\n%s", want, result.Stdout)
		}
	}
	server.AssertBearer(t, key)
	assertStoredChabToken(t, state.AuthPath, "local", key)
}

func TestBrowserLoginRejectsCustomAPIWithoutMatchingAppOrigin(t *testing.T) {
	for _, legacyAuth := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy_auth_%t", legacyAuth), func(t *testing.T) {
			state := testutil.NewState(t)
			if legacyAuth {
				testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(testutil.FakeKey("legacy_browser_guard")))
			}
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("browser login with API-only override made request to %s", r.URL)
			})
			result := testutil.RunCommandWith(t, testutil.Options{}, state.APIArgs(server, "login", "--web", "--no-prompt")...)
			if result.ExitCode != cli.ExitUsage || result.Err == nil || !strings.Contains(result.Stderr, "custom API URL requires --base-url") {
				t.Fatalf("browser login API-only result = %#v", result)
			}
			assertNotExists(t, state.ConfigPath)
			if !legacyAuth {
				assertNotExists(t, state.AuthPath)
			}
		})
	}
}

func TestPersistedAPIOnlySetupCannotLaterUseProductionBrowserIssuer(t *testing.T) {
	for _, command := range []string{"login", "setup"} {
		t.Run(command, func(t *testing.T) {
			state := testutil.NewState(t)
			testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{BaseURL: "http://127.0.0.1:9"}, true)
			key := testutil.FakeKey("saved_api_only_" + command)
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
				requireWhoamiRequest(t, r)
				fmt.Fprint(w, whoamiEnvelope())
			})
			args := []string{"--config", state.ConfigPath, "--auth-file", state.AuthPath, "--profile", "staging", "--api-base-url", server.APIBaseURL(), command, "--no-prompt"}
			if command == "login" {
				args = append(args, "--api-key")
			}
			first := testutil.RunCommandWith(t, testutil.Options{Stdin: strings.NewReader(key + "\n")}, args...)
			if first.ExitCode != cli.ExitSuccess || first.Err != nil || server.Count() != 1 {
				t.Fatalf("%s API-only setup result=%#v requests=%d", command, first, server.Count())
			}
			file, err := config.Load(state.ConfigPath)
			if err != nil {
				t.Fatal(err)
			}
			view, _, err := config.ResolveProfileView(file, "staging")
			if err != nil || view.BaseURL != config.DefaultBaseURL || view.APIBaseURL != server.APIBaseURL() {
				t.Fatalf("persisted mixed pair = %#v, err=%v", view, err)
			}
			for _, name := range []string{"local", "staging"} {
				switchResult := testutil.RunCommandWith(t, testutil.Options{}, state.Args("profile", "use", name)...)
				if switchResult.ExitCode != cli.ExitSuccess || switchResult.Err != nil {
					t.Fatalf("profile use %s result=%#v", name, switchResult)
				}
			}
			web := testutil.RunCommandWith(t, testutil.Options{}, state.Args("login", "--web", "--no-prompt")...)
			if web.ExitCode != cli.ExitUsage || web.Err == nil || !strings.Contains(web.Stderr, "custom API URL requires --base-url") || server.Count() != 1 {
				t.Fatalf("persisted API-only browser login result=%#v requests=%d", web, server.Count())
			}
		})
	}
}

func TestManualLoginRejectsInvalidIdentityBeforePersistence(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("login_invalid")
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		requireWhoamiRequest(t, r)
		fmt.Fprint(w, `{"data":{},"request_id":"req-empty-me"}`)
	})

	result := testutil.RunCommandWith(t, testutil.Options{
		Stdin: strings.NewReader(key + "\n"),
		Now:   func() time.Time { return commandNow },
	}, state.APIArgs(server, "login", "--api-key", "--no-prompt", "--yes", "--plain")...)
	if result.ExitCode != cli.ExitAPI || result.Stdout != "" || result.Err == nil {
		t.Fatalf("login result = %#v, want API/protocol failure before persistence", result)
	}
	if !strings.Contains(result.Stderr, "whoami response missing principal_type") {
		t.Fatalf("stderr missing validation context:\n%s", result.Stderr)
	}
	assertNotExists(t, state.AuthPath)
}

func TestWhoamiAndStatusExposeFlatChabTokenContext(t *testing.T) {
	state, key := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		requireWhoamiRequest(t, r)
		fmt.Fprint(w, whoamiEnvelope())
	})

	whoami := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server, "whoami", "--json")...)
	if whoami.ExitCode != cli.ExitSuccess || whoami.Err != nil || whoami.Stderr != "" {
		t.Fatalf("whoami result = %#v", whoami)
	}
	var whoamiBody map[string]any
	if err := json.Unmarshal([]byte(whoami.Stdout), &whoamiBody); err != nil {
		t.Fatalf("whoami JSON error = %v: %s", err, whoami.Stdout)
	}
	wantKeys := []string{"principal_type", "request_id", "scopes", "team_id", "token_controls", "token_id", "token_public_id"}
	if got := sortedMapKeys(whoamiBody); !reflect.DeepEqual(got, wantKeys) {
		t.Fatalf("whoami keys = %v, want %v", got, wantKeys)
	}
	if whoamiBody["principal_type"] != "team" || whoamiBody["team_id"] != float64(42) {
		t.Fatalf("whoami body = %#v", whoamiBody)
	}

	status := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server, "auth", "status", "--json")...)
	if status.ExitCode != cli.ExitSuccess || status.Err != nil || status.Stderr != "" {
		t.Fatalf("status result = %#v", status)
	}
	var statusBody struct {
		Usable bool `json:"usable"`
		Stored struct {
			PrincipalType string   `json:"principal_type"`
			TeamID        int64    `json:"team_id"`
			TokenPublicID string   `json:"token_public_id"`
			Scopes        []string `json:"scopes"`
		} `json:"stored"`
		Live struct {
			OK            bool   `json:"ok"`
			PrincipalType string `json:"principal_type"`
			TeamID        int64  `json:"team_id"`
			TokenPublicID string `json:"token_public_id"`
		} `json:"live"`
	}
	if err := json.Unmarshal([]byte(status.Stdout), &statusBody); err != nil {
		t.Fatalf("status JSON error = %v: %s", err, status.Stdout)
	}
	if !statusBody.Usable || statusBody.Stored.PrincipalType != "team" || statusBody.Stored.TeamID != 42 ||
		statusBody.Live.PrincipalType != "team" || statusBody.Live.TeamID != 42 ||
		statusBody.Stored.TokenPublicID != "ak_01HY0000000000000000000000" ||
		!statusBody.Live.OK {
		t.Fatalf("status body = %#v", statusBody)
	}
	server.AssertBearer(t, key)
}

func TestDoctorReportsChabRuntimeFindings(t *testing.T) {
	state, _ := configuredChabAuthState(t)
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cli/compatibility":
			fmt.Fprint(w, compatibilityEnvelope())
		case "/v1/me":
			requireWhoamiRequest(t, r)
			fmt.Fprint(w, whoamiEnvelope())
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	result := testutil.RunCommandWith(t, chabAuthOptions(), state.APIArgs(server, "doctor", "--json")...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("doctor result = %#v", result)
	}
	var report struct {
		Status   string           `json:"status"`
		Findings []findingSummary `json:"findings"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &report); err != nil {
		t.Fatalf("doctor JSON error = %v: %s", err, result.Stdout)
	}
	for _, id := range []string{"compatibility", "api.connectivity", "granted_scopes", "token_controls", "spending_allowance", "project_access"} {
		if !hasFinding(report.Findings, id, "ok") {
			t.Fatalf("doctor findings missing ok %s in %#v\nstdout=%s", id, report.Findings, result.Stdout)
		}
	}
	if report.Status != "pass" {
		t.Fatalf("doctor status = %q", report.Status)
	}
}

func configuredChabAuthState(t *testing.T) (testutil.State, string) {
	t.Helper()
	state := testutil.NewState(t)
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{Locale: "en"}, true)
	key := testutil.FakeKey("auth_chab")
	record := testutil.AuthRecord(key)
	record.PrincipalType = "team"
	record.TeamID = 42
	record.TokenID = "tok_internal_01HY0000000000000000000000"
	record.TokenPublicID = "ak_01HY0000000000000000000000"
	record.Scopes = []string{"api:projects:read", "api:credits:read"}
	validated := commandNow.UTC()
	record.LastValidatedAt = &validated
	testutil.WriteAuthProfile(t, state.AuthPath, "local", record)
	return state, key
}

func chabAuthOptions() testutil.Options {
	return testutil.Options{
		LookupEnv: testutil.HermeticEnv(nil),
		BootstrapFactory: func(rt config.Runtime, cmd *cobra.Command) (cmdutil.BootstrapClient, error) {
			return api.NewBootstrap(api.OptionsForBootstrap(rt, "1.0.0", false, cmd.ErrOrStderr()))
		},
	}
}
