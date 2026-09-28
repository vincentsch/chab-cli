package cli_test

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestProfileInspectionNoFilesAndOutputModes(t *testing.T) {
	state := testutil.NewState(t)
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "list human", args: []string{"profile", "list"}, want: "PROFILE"},
		{name: "list plain", args: []string{"profile", "list", "--plain"}, want: "PROFILE\tCURRENT\tSAVED\tBASE URL\tAPI BASE URL\tLOCALE\tDEFAULT OUTPUT\tLIMIT\tAUTH\tTEAM\tLAST VALIDATED"},
		{name: "list json", args: []string{"profile", "list", "--json"}, want: `"current_profile": "local"`},
		{name: "list jq", args: []string{"profile", "list", "--jq", ".current_profile"}, want: `"local"`},
		{name: "list template", args: []string{"profile", "list", "--template", "{{.current_profile}}"}, want: "local"},
		{name: "show human", args: []string{"profile", "show"}, want: "Saved"},
		{name: "show plain", args: []string{"profile", "show", "--plain"}, want: "Saved\tno"},
		{name: "show json", args: []string{"profile", "show", "--json"}, want: `"persisted": false`},
		{name: "show jq", args: []string{"profile", "show", "--jq", ".name"}, want: `"local"`},
		{name: "show template", args: []string{"profile", "show", "--template", "{{.name}}"}, want: "local"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := runLocalCommand(t, state, nil, test.args...)
			if result.ExitCode != cli.ExitSuccess || result.Err != nil {
				t.Fatalf("result = %#v", result)
			}
			if !strings.Contains(result.Stdout, test.want) {
				t.Fatalf("stdout missing %q:\n%s", test.want, result.Stdout)
			}
			if strings.Contains(test.name, "plain") {
				if !strings.Contains(result.Stderr, state.ConfigPath) {
					if strings.HasPrefix(test.name, "list") {
						t.Fatalf("plain list stderr missing config context: %q", result.Stderr)
					}
				}
			} else if result.Stderr != "" {
				t.Fatalf("stderr = %q", result.Stderr)
			}
			assertNotExists(t, state.ConfigPath)
			assertNotExists(t, state.AuthPath)
		})
	}

	plain := runLocalCommand(t, state, nil, "profile", "list", "--plain")
	const wantPlain = "PROFILE\tCURRENT\tSAVED\tBASE URL\tAPI BASE URL\tLOCALE\tDEFAULT OUTPUT\tLIMIT\tAUTH\tTEAM\tLAST VALIDATED\n" +
		"local\tyes\tno\thttps://www.chab.ai\thttps://www.chab.ai/v1\t-\ttable\t30\tnone\t-\t-\n"
	if plain.ExitCode != cli.ExitSuccess || plain.Stdout != wantPlain {
		t.Fatalf("no-file plain output = %q, want %q", plain.Stdout, wantPlain)
	}

	human := runLocalCommand(t, state, nil, "profile", "list")
	for _, want := range []string{
		"PROFILE", "CURRENT", "SAVED", "BASE URL", "API BASE URL", "LOCALE",
		"DEFAULT OUTPUT", "LIMIT", "AUTH", "TEAM", "LAST VALIDATED",
		"local", "yes", "no", "https://www.chab.ai", "https://www.chab.ai/v1", "table", "30",
	} {
		if !strings.Contains(human.Stdout, want) {
			t.Fatalf("human list missing %q:\n%s", want, human.Stdout)
		}
	}

	result := runLocalCommand(t, state, nil, "profile", "list", "--json")
	var list map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &list); err != nil {
		t.Fatal(err)
	}
	assertMapKeys(t, list, []string{"auth_path", "config_path", "current_profile", "profiles"})
	profiles := list["profiles"].([]any)
	if len(profiles) != 1 {
		t.Fatalf("profiles = %#v", profiles)
	}
	profile := profiles[0].(map[string]any)
	assertMapKeys(t, profile, []string{"api_base_url", "base_url", "default_output", "locale", "name", "persisted", "project_list_limit", "selected", "stored_auth"})
	stored := profile["stored_auth"].(map[string]any)
	assertMapKeys(t, stored, []string{"display_id", "key_name", "last_validated_at", "present", "team_display_id", "team_name"})
	for _, key := range []string{"display_id", "key_name", "last_validated_at", "team_display_id", "team_name"} {
		if stored[key] != nil {
			t.Fatalf("stored_auth.%s = %#v, want null", key, stored[key])
		}
	}
}

func TestAuthOnlyLegacyInspectionMatchesRequestDestination(t *testing.T) {
	state := testutil.NewState(t)
	key := testutil.FakeKey("legacy_auth_only_destination")
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(key))
	for _, args := range [][]string{
		{"profile", "show", "--json"},
		{"profile", "list", "--json"},
		{"config", "list", "--json"},
	} {
		result := runLocalCommand(t, state, nil, args...)
		if result.ExitCode != cli.ExitSuccess || result.Err != nil {
			t.Fatalf("%v result = %#v", args, result)
		}
		if !strings.Contains(result.Stdout, "http://localhost/v1") || !strings.Contains(result.Stdout, "http://localhost") {
			t.Fatalf("%v inspection differs from effective legacy destination: %s", args, result.Stdout)
		}
		if strings.Contains(result.Stdout, key) {
			t.Fatalf("%v leaked stored credential", args)
		}
	}
}

func TestAuthOnlyNamedProfileUsesAuthEnvForEffectiveDestination(t *testing.T) {
	state := testutil.NewState(t)
	testutil.WriteAuthProfile(t, state.AuthPath, "ci", testutil.AuthRecord(testutil.FakeKey("named_auth_only")))
	env := map[string]string{"CHAB_PROFILE": "ci"}
	for _, args := range [][]string{{"auth", "env", "--json"}, {"profile", "show", "--json"}} {
		result := runLocalCommand(t, state, env, args...)
		if result.ExitCode != cli.ExitSuccess || !strings.Contains(result.Stdout, "http://localhost/v1") {
			t.Fatalf("%v named auth-only destination = %#v", args, result)
		}
	}
	list := runLocalCommand(t, state, env, "config", "list", "--json")
	if list.ExitCode != cli.ExitSuccess || !strings.Contains(list.Stdout, `"key": "profiles.local.api_base_url"`) ||
		!strings.Contains(list.Stdout, "https://www.chab.ai/v1") || strings.Contains(list.Stdout, `"key": "profiles.ci.api_base_url"`) {
		t.Fatalf("config list should describe built-in local defaults, not CHAB_PROFILE runtime: %#v", list)
	}
}

func TestProfileInspectionSelectionStoredAuthAndSorting(t *testing.T) {
	state := testutil.NewState(t)
	testutil.WriteConfigProfile(t, state.ConfigPath, "zeta", config.Profile{BaseURL: "https://zeta.example.test"}, true)
	testutil.WriteConfigProfile(t, state.ConfigPath, "production.eu", config.Profile{BaseURL: "https://prod.example.test", Locale: "de"}, false)
	checkedAt := time.Date(2026, 7, 27, 20, 0, 0, 0, time.UTC)
	storedKey := testutil.FakeKey("profile_metadata")
	testutil.WriteAuthProfile(t, state.AuthPath, "production.eu", auth.ProfileAuth{
		APIKey:             storedKey,
		DisplayID:          "",
		KeyName:            "",
		TeamDisplayID:      "",
		TeamName:           "",
		LastValidatedAt:    &checkedAt,
		KeyPreset:          "default",
		KeyExpirationState: "active",
	})
	authOnlyKey := testutil.FakeKey("auth_only")
	testutil.WriteAuthProfile(t, state.AuthPath, "auth-only", testutil.AuthRecord(authOnlyKey))

	configSelected := runLocalCommand(t, state, nil, "profile", "show", "--json")
	if configSelected.ExitCode != cli.ExitSuccess ||
		!strings.Contains(configSelected.Stdout, `"name": "zeta"`) ||
		!strings.Contains(configSelected.Stdout, `"selected": true`) ||
		!strings.Contains(configSelected.Stdout, `"persisted": true`) {
		t.Fatalf("config-selected profile result = %#v", configSelected)
	}

	env := map[string]string{
		"CHAB_PROFILE": "unsaved",
		"CHAB_API_KEY": testutil.FakeKey("environment_shadow"),
	}
	result := runLocalCommand(t, state, env, "profile", "list", "--json")
	if result.ExitCode != cli.ExitSuccess || result.Stderr != "" {
		t.Fatalf("list result = %#v", result)
	}
	var list struct {
		CurrentProfile string `json:"current_profile"`
		Profiles       []struct {
			Name       string `json:"name"`
			Selected   bool   `json:"selected"`
			Persisted  bool   `json:"persisted"`
			StoredAuth struct {
				Present         bool    `json:"present"`
				DisplayID       *string `json:"display_id"`
				KeyName         *string `json:"key_name"`
				TeamDisplayID   *string `json:"team_display_id"`
				TeamName        *string `json:"team_name"`
				LastValidatedAt *string `json:"last_validated_at"`
			} `json:"stored_auth"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &list); err != nil {
		t.Fatal(err)
	}
	if list.CurrentProfile != "unsaved" {
		t.Fatalf("current profile = %q", list.CurrentProfile)
	}
	var names []string
	for _, profile := range list.Profiles {
		names = append(names, profile.Name)
	}
	if want := []string{"production.eu", "unsaved", "zeta"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("profile names = %v, want %v", names, want)
	}
	for _, secret := range []string{storedKey, authOnlyKey, env["CHAB_API_KEY"]} {
		if strings.Contains(result.Stdout, secret) {
			t.Fatal("list exposed credential state")
		}
	}
	if strings.Contains(result.Stdout, "auth-only") {
		t.Fatal("list included an auth-only profile")
	}
	production := list.Profiles[0]
	if !production.Persisted || production.Selected || !production.StoredAuth.Present ||
		production.StoredAuth.DisplayID == nil || *production.StoredAuth.DisplayID != "" ||
		production.StoredAuth.KeyName == nil || *production.StoredAuth.KeyName != "" ||
		production.StoredAuth.TeamDisplayID == nil || *production.StoredAuth.TeamDisplayID != "" ||
		production.StoredAuth.TeamName == nil || *production.StoredAuth.TeamName != "" ||
		production.StoredAuth.LastValidatedAt == nil || *production.StoredAuth.LastValidatedAt != "2026-07-27T20:00:00Z" {
		t.Fatalf("production stored auth shape was not preserved")
	}

	for _, args := range [][]string{
		{"profile", "list"},
		{"profile", "list", "--plain"},
	} {
		rendered := runLocalCommand(t, state, env, args...)
		if rendered.ExitCode != cli.ExitSuccess {
			t.Fatalf("%v result = %#v", args, rendered)
		}
		for _, want := range []string{"production.eu", "unsaved", "yes", "no", "table"} {
			if !strings.Contains(rendered.Stdout, want) {
				t.Fatalf("%v stdout missing %q:\n%s", args, want, rendered.Stdout)
			}
		}
		if strings.Contains(rendered.Stdout+rendered.Stderr, storedKey) ||
			strings.Contains(rendered.Stdout+rendered.Stderr, authOnlyKey) ||
			strings.Contains(rendered.Stdout+rendered.Stderr, env["CHAB_API_KEY"]) {
			t.Fatalf("%v exposed credential state", args)
		}
	}

	override := runLocalCommand(t, state, env, "--profile", "flagged", "profile", "show", "--json")
	if override.ExitCode != cli.ExitSuccess || !strings.Contains(override.Stdout, `"name": "flagged"`) ||
		!strings.Contains(override.Stdout, `"selected": true`) || !strings.Contains(override.Stdout, `"persisted": false`) {
		t.Fatalf("flag selection result = %#v", override)
	}

	authOnly := runLocalCommand(t, state, nil, "profile", "show", "auth-only", "--json")
	if authOnly.ExitCode != cli.ExitSuccess || !strings.Contains(authOnly.Stdout, `"persisted": false`) ||
		!strings.Contains(authOnly.Stdout, `"present": true`) {
		t.Fatalf("auth-only show result = %#v", authOnly)
	}
}

func TestProfileInspectionRejectsMalformedAndInvalidLocalState(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, state testutil.State)
		args  []string
	}{
		{name: "malformed config", setup: func(t *testing.T, state testutil.State) {
			testutil.WriteMalformedConfig(t, state.ConfigPath)
		}, args: []string{"profile", "list"}},
		{name: "malformed auth", setup: func(t *testing.T, state testutil.State) {
			testutil.WriteMalformedAuth(t, state.AuthPath)
		}, args: []string{"profile", "show"}},
		{name: "unsupported config version", setup: func(t *testing.T, state testutil.State) {
			writeLocalFixture(t, state.ConfigPath, "version: 2\nprofiles: {}\n")
		}, args: []string{"profile", "list"}},
		{name: "unsupported auth version", setup: func(t *testing.T, state testutil.State) {
			writeLocalFixture(t, state.AuthPath, "{\"version\":2,\"profiles\":{}}\n")
		}, args: []string{"profile", "show"}},
		{name: "invalid profile value", setup: func(t *testing.T, state testutil.State) {
			writeLocalFixture(t, state.ConfigPath, "version: 1\ncurrent_profile: local\nprofiles:\n  local:\n    base_url: ftp://bad.example\n")
		}, args: []string{"profile", "show", "local"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := testutil.NewState(t)
			test.setup(t, state)
			result := runLocalCommand(t, state, nil, test.args...)
			if result.ExitCode != cli.ExitUsage || result.Stdout != "" || result.Stderr == "" {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestProfileInspectionTransformErrorsSanitizeStoredMetadataControls(t *testing.T) {
	state := testutil.NewState(t)
	testutil.WriteConfigProfile(t, state.ConfigPath, "local", config.Profile{
		BaseURL: "https://local.example.test",
	}, true)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", auth.ProfileAuth{
		APIKey:    testutil.FakeKey("controlled_metadata"),
		DisplayID: "left\x1b[31mred\x1b[0mright",
	})

	result := runLocalCommand(
		t,
		state,
		nil,
		"profile",
		"show",
		"local",
		"--jq",
		`.stored_auth.display_id | error(.)`,
	)
	if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
		t.Fatalf("result = %#v", result)
	}
	if safe := string(output.SanitizeControlBytes([]byte(result.Stderr))); safe != result.Stderr {
		t.Fatalf("stderr contains terminal controls: %q", result.Stderr)
	}
	for _, want := range []string{"left", "red", "right"} {
		if !strings.Contains(result.Stderr, want) {
			t.Fatalf("stderr missing %q: %q", want, result.Stderr)
		}
	}
}

func runLocalCommand(t *testing.T, state testutil.State, env map[string]string, args ...string) testutil.Result {
	t.Helper()
	return testutil.RunCommandWith(t, testutil.Options{
		LookupEnv:        testutil.HermeticEnv(env),
		StdinIsTerminal:  func() bool { return false },
		StdoutIsTerminal: func() bool { return false },
	}, state.Args(args...)...)
}

func writeLocalFixture(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readLocalFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertMapKeys(t *testing.T, value map[string]any, want []string) {
	t.Helper()
	got := make([]string, 0, len(value))
	for key := range value {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
}

func assertLocalFilesAbsent(t *testing.T, state testutil.State) {
	t.Helper()
	for _, path := range []string{state.ConfigPath, state.AuthPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s exists or stat failed: %v", path, err)
		}
	}
}
