package cli_test

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestProfileCreateValidationOwnershipAndWrites(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "invalid name", args: []string{"profile", "create", "Bad Name", "--base-url", "https://example.test"}},
		{name: "missing base URL", args: []string{"profile", "create", "staging"}},
		{name: "empty base URL", args: []string{"profile", "create", "staging", "--base-url", ""}},
		{name: "invalid base URL", args: []string{"profile", "create", "staging", "--base-url", "ftp://example.test"}},
		{name: "empty API base URL", args: []string{"profile", "create", "staging", "--base-url", "https://example.test", "--api-base-url", ""}},
		{name: "invalid API base URL", args: []string{"profile", "create", "staging", "--base-url", "https://example.test", "--api-base-url", "ftp://api.example.test"}},
		{name: "invalid locale", args: []string{"profile", "create", "staging", "--base-url", "https://example.test", "--locale", "fr"}},
		{name: "empty locale", args: []string{"profile", "create", "staging", "--base-url", "https://example.test", "--locale", ""}},
		{name: "invalid limit", args: []string{"profile", "create", "staging", "--base-url", "https://example.test", "--project-list-limit", "0"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := testutil.NewState(t)
			result := runLocalCommand(t, state, nil, test.args...)
			if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
				t.Fatalf("result = %#v", result)
			}
			assertLocalFilesAbsent(t, state)
		})
	}

	t.Run("validation precedes local state reads", func(t *testing.T) {
		state := testutil.NewState(t)
		testutil.WriteMalformedConfig(t, state.ConfigPath)
		testutil.WriteMalformedAuth(t, state.AuthPath)
		beforeConfig := readLocalFile(t, state.ConfigPath)
		beforeAuth := readLocalFile(t, state.AuthPath)
		result := runLocalCommand(t, state, nil, "profile", "create", "staging", "--base-url", "ftp://example.test")
		if result.ExitCode != cli.ExitUsage || result.Stdout != "" ||
			strings.Contains(result.Stderr, "could not parse") {
			t.Fatalf("result = %#v", result)
		}
		if !reflect.DeepEqual(readLocalFile(t, state.ConfigPath), beforeConfig) ||
			!reflect.DeepEqual(readLocalFile(t, state.AuthPath), beforeAuth) {
			t.Fatal("validation failure changed local files")
		}
	})

	t.Run("auth-only ownership", func(t *testing.T) {
		state := testutil.NewState(t)
		testutil.WriteAuthProfile(t, state.AuthPath, "staging", testutil.AuthRecord(testutil.FakeKey("auth_only_owner")))
		beforeAuth := readLocalFile(t, state.AuthPath)
		result := runLocalCommand(t, state, nil, "profile", "create", "staging", "--base-url", "https://example.test")
		if result.ExitCode != cli.ExitUsage || result.Stdout != "" ||
			!strings.Contains(result.Stderr, "chab logout --profile staging") {
			t.Fatalf("result = %#v", result)
		}
		assertNotExists(t, state.ConfigPath)
		if !reflect.DeepEqual(readLocalFile(t, state.AuthPath), beforeAuth) {
			t.Fatal("auth-only refusal changed auth")
		}
	})

	t.Run("duplicate config and both-present", func(t *testing.T) {
		for _, withAuth := range []bool{false, true} {
			state := testutil.NewState(t)
			testutil.WriteConfigProfile(t, state.ConfigPath, "staging", config.Profile{BaseURL: "https://old.example.test"}, true)
			if withAuth {
				testutil.WriteAuthProfile(t, state.AuthPath, "staging", testutil.AuthRecord(testutil.FakeKey("duplicate_owner")))
			}
			beforeConfig := readLocalFile(t, state.ConfigPath)
			var beforeAuth []byte
			if withAuth {
				beforeAuth = readLocalFile(t, state.AuthPath)
			}
			result := runLocalCommand(t, state, nil, "profile", "create", "staging", "--base-url", "https://new.example.test")
			if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
				t.Fatalf("withAuth=%v result = %#v", withAuth, result)
			}
			if !reflect.DeepEqual(readLocalFile(t, state.ConfigPath), beforeConfig) {
				t.Fatal("duplicate refusal changed config")
			}
			if withAuth && !reflect.DeepEqual(readLocalFile(t, state.AuthPath), beforeAuth) {
				t.Fatal("duplicate refusal changed auth")
			}
		}
	})

	t.Run("malformed auth", func(t *testing.T) {
		state := testutil.NewState(t)
		testutil.WriteMalformedAuth(t, state.AuthPath)
		before := readLocalFile(t, state.AuthPath)
		result := runLocalCommand(t, state, nil, "profile", "create", "staging", "--base-url", "https://example.test")
		if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
			t.Fatalf("result = %#v", result)
		}
		assertNotExists(t, state.ConfigPath)
		if !reflect.DeepEqual(readLocalFile(t, state.AuthPath), before) {
			t.Fatal("malformed auth changed")
		}
	})

	t.Run("first and later profiles", func(t *testing.T) {
		state := testutil.NewState(t)
		first := runLocalCommand(t, state, nil, "profile", "create", "staging",
			"--base-url", "https://staging.example.test/root/",
			"--locale", "de",
			"--project-list-limit", "45",
			"--plain",
		)
		if first.ExitCode != cli.ExitSuccess || !strings.Contains(first.Stdout, "https://staging.example.test/root") {
			t.Fatalf("first create = %#v", first)
		}
		assertNotExists(t, state.AuthPath)
		file, err := config.Load(state.ConfigPath)
		if err != nil {
			t.Fatal(err)
		}
		if file.CurrentProfile != "staging" {
			t.Fatalf("first current profile = %q", file.CurrentProfile)
		}
		view, _, err := config.ResolveProfileView(file, "staging")
		if err != nil {
			t.Fatal(err)
		}
		if view.BaseURL != "https://staging.example.test/root" ||
			view.APIBaseURL != "https://staging.example.test/root/v1" ||
			view.Locale != "de" || view.ProjectListLimit != 45 {
			t.Fatalf("first profile view = %#v", view)
		}
		second := runLocalCommand(t, state, nil, "profile", "create", "other",
			"--base-url", "https://other.example.test/",
			"--api-base-url", "https://api.other.example.test/v1/",
			"--locale", "en",
			"--project-list-limit", "12",
		)
		if second.ExitCode != cli.ExitSuccess {
			t.Fatalf("second create = %#v", second)
		}
		file, err = config.Load(state.ConfigPath)
		if err != nil {
			t.Fatal(err)
		}
		if file.CurrentProfile != "staging" {
			t.Fatalf("later create changed selection to %q", file.CurrentProfile)
		}
		other, _, err := config.ResolveProfileView(file, "other")
		if err != nil {
			t.Fatal(err)
		}
		if other.BaseURL != "https://other.example.test" ||
			other.APIBaseURL != "https://api.other.example.test/v1" ||
			other.Locale != "en" || other.ProjectListLimit != 12 {
			t.Fatalf("later profile view = %#v", other)
		}
	})

	t.Run("preserves comments and unknown keys", func(t *testing.T) {
		state := testutil.NewState(t)
		writeLocalFixture(t, state.ConfigPath, `# retained comment
version: 1
current_profile: existing
extension: keep
profiles:
  existing:
    base_url: https://existing.example.test
    custom: keep-profile
`)
		result := runLocalCommand(t, state, nil, "profile", "create", "new", "--base-url", "https://new.example.test")
		if result.ExitCode != cli.ExitSuccess {
			t.Fatalf("result = %#v", result)
		}
		data := string(readLocalFile(t, state.ConfigPath))
		for _, want := range []string{"# retained comment", "extension: keep", "custom: keep-profile", "new:"} {
			if !strings.Contains(data, want) {
				t.Fatalf("config missing %q:\n%s", want, data)
			}
		}
	})
}

func TestProfileUseChangesOnlyConfig(t *testing.T) {
	state := testutil.NewState(t)
	testutil.WriteConfigProfile(t, state.ConfigPath, "alpha", config.Profile{BaseURL: "https://alpha.example.test"}, true)
	testutil.WriteConfigProfile(t, state.ConfigPath, "beta", config.Profile{BaseURL: "https://beta.example.test"}, false)
	testutil.WriteMalformedAuth(t, state.AuthPath)
	beforeAuth := readLocalFile(t, state.AuthPath)
	result := runLocalCommand(t, state, nil, "profile", "use", "beta", "--plain")
	if result.ExitCode != cli.ExitSuccess || !strings.Contains(result.Stdout, "beta") {
		t.Fatalf("result = %#v", result)
	}
	file, err := config.Load(state.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if file.CurrentProfile != "beta" {
		t.Fatalf("current profile = %q", file.CurrentProfile)
	}
	if !reflect.DeepEqual(readLocalFile(t, state.AuthPath), beforeAuth) {
		t.Fatal("profile use read/write path changed auth bytes")
	}

	beforeConfig := readLocalFile(t, state.ConfigPath)
	missing := runLocalCommand(t, state, nil, "profile", "use", "missing")
	if missing.ExitCode != cli.ExitUsage || missing.Stdout != "" ||
		!reflect.DeepEqual(readLocalFile(t, state.ConfigPath), beforeConfig) {
		t.Fatalf("missing use = %#v", missing)
	}

	preserved := testutil.NewState(t)
	writeLocalFixture(t, preserved.ConfigPath, `# keep
version: 1
current_profile: alpha
extension: keep-top
profiles:
  alpha:
    base_url: https://alpha.example.test
    extension: keep-alpha
  beta:
    base_url: https://beta.example.test
    extension: keep-beta
`)
	result = runLocalCommand(t, preserved, nil, "profile", "use", "beta")
	if result.ExitCode != cli.ExitSuccess {
		t.Fatalf("preserving use = %#v", result)
	}
	data := string(readLocalFile(t, preserved.ConfigPath))
	for _, want := range []string{"# keep", "extension: keep-top", "extension: keep-alpha", "extension: keep-beta"} {
		if !strings.Contains(data, want) {
			t.Fatalf("profile use dropped %q:\n%s", want, data)
		}
	}
}

func TestProfileAndConfigLeavesStayLocal(t *testing.T) {
	server := testutil.NewAPIServer(t, testutil.FailOnContact(t))
	state := testutil.NewState(t)
	for _, args := range [][]string{
		{"profile", "create", "local", "--base-url", server.URL, "--api-base-url", server.APIBaseURL()},
		{"profile", "list", "--json"},
		{"profile", "show", "local", "--json"},
		{"profile", "use", "local"},
		{"config", "path", "--json"},
		{"config", "list", "--json"},
		{"config", "get", "profiles.local.api_base_url", "--json"},
		{"config", "set", "profiles.local.locale", "de"},
		{"profile", "delete", "local", "--yes"},
	} {
		result := runLocalCommand(t, state, nil, args...)
		if result.ExitCode != cli.ExitSuccess {
			t.Fatalf("%v result = %#v", args, result)
		}
	}
	server.AssertNoRequests(t)
	assertNotExists(t, state.AuthPath)
}

func TestProfileDeleteConfirmationLifecycleAndPreservation(t *testing.T) {
	t.Run("nonexistent", func(t *testing.T) {
		state := testutil.NewState(t)
		result := runLocalCommand(t, state, nil, "profile", "delete", "missing", "--yes")
		if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
			t.Fatalf("result = %#v", result)
		}
		assertLocalFilesAbsent(t, state)
	})

	t.Run("deny and unavailable prompt", func(t *testing.T) {
		for _, test := range []struct {
			name     string
			stdin    string
			terminal bool
			extra    []string
		}{
			{name: "interactive deny", stdin: "n\n", terminal: true},
			{name: "interactive EOF", stdin: "", terminal: true},
			{name: "non-interactive", stdin: "y\n", terminal: false},
			{name: "no prompt", stdin: "y\n", terminal: true, extra: []string{"--no-prompt"}},
		} {
			t.Run(test.name, func(t *testing.T) {
				state := testutil.NewState(t)
				testutil.WriteConfigProfile(t, state.ConfigPath, "alpha", config.Profile{BaseURL: "https://alpha.example.test"}, true)
				before := readLocalFile(t, state.ConfigPath)
				args := state.Args("profile", "delete", "alpha")
				args = append(args, test.extra...)
				result := testutil.RunCommandWith(t, testutil.Options{
					LookupEnv:       testutil.HermeticEnv(nil),
					Stdin:           strings.NewReader(test.stdin),
					StdinIsTerminal: func() bool { return test.terminal },
				}, args...)
				if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
					t.Fatalf("result = %#v", result)
				}
				if !reflect.DeepEqual(readLocalFile(t, state.ConfigPath), before) {
					t.Fatal("denied delete changed config")
				}
			})
		}
	})

	t.Run("interactive accept and sorted fallback", func(t *testing.T) {
		state := testutil.NewState(t)
		testutil.WriteConfigProfile(t, state.ConfigPath, "zeta", config.Profile{BaseURL: "https://zeta.example.test"}, true)
		testutil.WriteConfigProfile(t, state.ConfigPath, "beta", config.Profile{BaseURL: "https://beta.example.test"}, false)
		testutil.WriteConfigProfile(t, state.ConfigPath, "alpha", config.Profile{BaseURL: "https://alpha.example.test"}, false)
		testutil.WriteAuthProfile(t, state.AuthPath, "zeta", testutil.AuthRecord(testutil.FakeKey("delete_accept")))
		result := testutil.RunCommandWith(t, testutil.Options{
			LookupEnv:       testutil.HermeticEnv(nil),
			Stdin:           strings.NewReader("y\n"),
			StdinIsTerminal: func() bool { return true },
		}, state.Args("profile", "delete", "zeta", "--debug")...)
		if result.ExitCode != cli.ExitSuccess || !strings.Contains(result.Stdout, "Deleted") ||
			!strings.Contains(result.Stdout, "Server-side API key revocation") ||
			!strings.Contains(result.Stderr, `Delete profile "zeta"?`) {
			t.Fatalf("result = %#v", result)
		}
		file, err := config.Load(state.ConfigPath)
		if err != nil {
			t.Fatal(err)
		}
		if file.CurrentProfile != "alpha" {
			t.Fatalf("fallback current = %q", file.CurrentProfile)
		}
		authFile, _, err := auth.Load(state.AuthPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := authFile.Profiles["zeta"]; ok {
			t.Fatal("deleted auth record remains")
		}
	})

	t.Run("dotted config-only target repairs dangling current", func(t *testing.T) {
		state := testutil.NewState(t)
		writeLocalFixture(t, state.ConfigPath, `version: 1
current_profile: missing
profiles:
  target.eu:
    base_url: https://target.example.test
  alpha:
    base_url: https://alpha.example.test
`)
		result := runLocalCommand(t, state, nil, "profile", "delete", "target.eu", "--yes")
		if result.ExitCode != cli.ExitSuccess {
			t.Fatalf("result = %#v", result)
		}
		file, err := config.Load(state.ConfigPath)
		if err != nil {
			t.Fatal(err)
		}
		if file.CurrentProfile != "alpha" {
			t.Fatalf("repaired current = %q", file.CurrentProfile)
		}
		if _, ok := file.Profiles["target.eu"]; ok {
			t.Fatal("dotted target remains in config")
		}
		assertNotExists(t, state.AuthPath)
	})

	t.Run("auth-only and final safe removal", func(t *testing.T) {
		authOnly := testutil.NewState(t)
		testutil.WriteAuthProfile(t, authOnly.AuthPath, "orphan", testutil.AuthRecord(testutil.FakeKey("delete_auth_only")))
		result := runLocalCommand(t, authOnly, nil, "profile", "delete", "orphan", "--yes")
		if result.ExitCode != cli.ExitSuccess {
			t.Fatalf("auth-only delete = %#v", result)
		}
		assertNotExists(t, authOnly.ConfigPath)

		final := testutil.NewState(t)
		testutil.WriteConfigProfile(t, final.ConfigPath, "only", config.Profile{BaseURL: "https://only.example.test"}, true)
		result = runLocalCommand(t, final, nil, "profile", "delete", "only", "--yes")
		if result.ExitCode != cli.ExitSuccess {
			t.Fatalf("final delete = %#v", result)
		}
		assertNotExists(t, final.ConfigPath)
	})

	t.Run("final custom data refuses before confirmation", func(t *testing.T) {
		state := testutil.NewState(t)
		writeLocalFixture(t, state.ConfigPath, "# keep\nversion: 1\ncurrent_profile: only\nextension: keep\nprofiles:\n  only:\n    base_url: https://only.example.test\n")
		before := readLocalFile(t, state.ConfigPath)
		result := runLocalCommand(t, state, nil, "profile", "delete", "only")
		if result.ExitCode != cli.ExitUsage || result.Stdout != "" || strings.Contains(result.Stderr, "Delete profile") {
			t.Fatalf("result = %#v", result)
		}
		if !reflect.DeepEqual(readLocalFile(t, state.ConfigPath), before) {
			t.Fatal("unsafe final delete changed config")
		}
	})

	t.Run("final nested comments refuse before confirmation", func(t *testing.T) {
		for _, test := range []struct {
			name string
			body string
		}{
			{
				name: "before profile field",
				body: "version: 1\ncurrent_profile: only\nprofiles:\n  only:\n    # keep field context\n    base_url: https://only.example.test\n",
			},
			{
				name: "inline profile field",
				body: "version: 1\ncurrent_profile: only\nprofiles:\n  only:\n    base_url: https://only.example.test # keep inline context\n",
			},
			{
				name: "inside defaults",
				body: "version: 1\ncurrent_profile: only\nprofiles:\n  only:\n    base_url: https://only.example.test\n    defaults:\n      # keep limit context\n      project_list_limit: 30\n",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				state := testutil.NewState(t)
				writeLocalFixture(t, state.ConfigPath, test.body)
				before := readLocalFile(t, state.ConfigPath)
				result := runLocalCommand(t, state, nil, "profile", "delete", "only")
				if result.ExitCode != cli.ExitUsage || result.Stdout != "" ||
					strings.Contains(result.Stderr, "Delete profile") {
					t.Fatalf("result = %#v", result)
				}
				if !reflect.DeepEqual(readLocalFile(t, state.ConfigPath), before) {
					t.Fatal("nested-comment final delete changed config")
				}
			})
		}
	})
}

func TestProfileDeleteTwoFileFailureOrdering(t *testing.T) {
	t.Run("auth failure keeps config", func(t *testing.T) {
		state := testutil.NewState(t)
		testutil.WriteConfigProfile(t, state.ConfigPath, "target", config.Profile{BaseURL: "https://target.example.test"}, true)
		testutil.WriteConfigProfile(t, state.ConfigPath, "remaining", config.Profile{BaseURL: "https://remaining.example.test"}, false)
		writeLocalFixture(t, state.AuthPath, `{
  "version": 1,
  "profiles": {
    "target": {"api_key": "opaque-target"},
    "remaining": {"api_key": ""}
  }
}
`)
		beforeConfig := readLocalFile(t, state.ConfigPath)
		beforeAuth := readLocalFile(t, state.AuthPath)
		result := runLocalCommand(t, state, nil, "profile", "delete", "target", "--yes")
		if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
			t.Fatalf("result = %#v", result)
		}
		if !reflect.DeepEqual(readLocalFile(t, state.ConfigPath), beforeConfig) ||
			!reflect.DeepEqual(readLocalFile(t, state.AuthPath), beforeAuth) {
			t.Fatal("auth-write failure changed local files")
		}
	})

	t.Run("config failure reports partial cleanup", func(t *testing.T) {
		state := testutil.NewState(t)
		writeLocalFixture(t, state.ConfigPath, `version: 1
current_profile: target
profiles:
  target:
    base_url: https://target.example.test
  remaining:
    base_url: https://remaining.example.test
    locale: fr
`)
		testutil.WriteAuthProfile(t, state.AuthPath, "target", testutil.AuthRecord(testutil.FakeKey("partial_cleanup")))
		beforeConfig := readLocalFile(t, state.ConfigPath)
		result := runLocalCommand(t, state, nil, "profile", "delete", "target", "--yes")
		if result.ExitCode != cli.ExitUsage || result.Stdout != "" ||
			!strings.Contains(result.Stderr, "local stored credential was removed") ||
			!strings.Contains(result.Stderr, "config write failed") {
			t.Fatalf("result = %#v", result)
		}
		if !reflect.DeepEqual(readLocalFile(t, state.ConfigPath), beforeConfig) {
			t.Fatal("partial cleanup changed config bytes")
		}
		authFile, _, err := auth.Load(state.AuthPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := authFile.Profiles["target"]; ok {
			t.Fatal("target auth was not removed before config failure")
		}
	})
}

func TestLocalMutationOutputModesAndPreflight(t *testing.T) {
	for _, command := range []string{"profile create", "profile use", "profile delete", "config set"} {
		t.Run(command, func(t *testing.T) {
			run := func(t *testing.T, mode ...string) testutil.Result {
				t.Helper()
				state := testutil.NewState(t)
				var args []string
				switch command {
				case "profile create":
					args = []string{"profile", "create", "new", "--base-url", "https://new.example.test"}
				case "profile use":
					testutil.WriteConfigProfile(t, state.ConfigPath, "new", config.Profile{BaseURL: "https://new.example.test"}, true)
					args = []string{"profile", "use", "new"}
				case "profile delete":
					testutil.WriteConfigProfile(t, state.ConfigPath, "new", config.Profile{BaseURL: "https://new.example.test"}, true)
					args = []string{"profile", "delete", "new", "--yes"}
				case "config set":
					testutil.WriteConfigProfile(t, state.ConfigPath, "new", config.Profile{BaseURL: "https://new.example.test"}, true)
					args = []string{"config", "set", "profiles.new.locale", "de"}
				}
				args = append(args, mode...)
				return runLocalCommand(t, state, nil, args...)
			}

			human := run(t)
			plain := run(t, "--plain")
			jsonMode := run(t, "--json")
			for name, result := range map[string]testutil.Result{"human": human, "plain": plain, "json": jsonMode} {
				if result.ExitCode != cli.ExitSuccess || result.Stdout == "" {
					t.Fatalf("%s result = %#v", name, result)
				}
			}
			if strings.HasPrefix(strings.TrimSpace(jsonMode.Stdout), "{") || strings.HasPrefix(strings.TrimSpace(jsonMode.Stdout), "[") {
				t.Fatalf("--json unexpectedly changed mutation shape: %q", jsonMode.Stdout)
			}
			if jsonMode.Stdout != human.Stdout || jsonMode.Stderr != human.Stderr {
				t.Fatalf("--json mutation output differs from human: human=%#v json=%#v", human, jsonMode)
			}
			if strings.ContainsAny(plain.Stdout+plain.Stderr, "\x1b\x07") {
				t.Fatalf("plain output contains terminal controls: %#v", plain)
			}

			for _, mode := range [][]string{{"--jq", "."}, {"--template", "{{.}}"}} {
				state := testutil.NewState(t)
				testutil.WriteMalformedConfig(t, state.ConfigPath)
				testutil.WriteMalformedAuth(t, state.AuthPath)
				args := strings.Fields(command)
				switch command {
				case "profile create":
					args = append(args, "new", "--base-url", "https://new.example.test")
				case "profile use", "profile delete":
					args = append(args, "new")
				case "config set":
					args = append(args, "profiles.new.locale", "de")
				}
				args = append(args, mode...)
				beforeConfig := readLocalFile(t, state.ConfigPath)
				beforeAuth := readLocalFile(t, state.AuthPath)
				result := runLocalCommand(t, state, nil, args...)
				if result.ExitCode != cli.ExitUsage || result.Stdout != "" ||
					(!strings.Contains(result.Stderr, "--jq is not supported") && !strings.Contains(result.Stderr, "--template is not supported")) {
					t.Fatalf("%v result = %#v", mode, result)
				}
				if !bytes.Equal(readLocalFile(t, state.ConfigPath), beforeConfig) ||
					!bytes.Equal(readLocalFile(t, state.AuthPath), beforeAuth) {
					t.Fatal("output preflight changed local files")
				}
			}
		})
	}

	for _, family := range []string{"profile", "config"} {
		for _, mode := range [][]string{{"--plain"}, {"--jq", "."}, {"--template", "{{.}}"}} {
			result := testutil.RunCommandWith(t, testutil.Options{LookupEnv: testutil.HermeticEnv(nil)}, append([]string{family}, mode...)...)
			if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
				t.Fatalf("%s %v result = %#v", family, mode, result)
			}
		}
		help := testutil.RunCommandWith(t, testutil.Options{LookupEnv: testutil.HermeticEnv(nil)}, family, "--help")
		if help.ExitCode != cli.ExitSuccess || help.Stderr != "" {
			t.Fatalf("%s help = %#v", family, help)
		}
	}
}

func TestProfileDeleteRemovesTargetOwnedUnknownDataOnly(t *testing.T) {
	state := testutil.NewState(t)
	writeLocalFixture(t, state.ConfigPath, `version: 1
current_profile: target
extension: keep-top
profiles:
  target:
    base_url: https://target.example.test
    target_extension: remove
  remaining:
    base_url: https://remaining.example.test
    remaining_extension: keep
`)
	writeLocalFixture(t, state.AuthPath, `{
  "version": 1,
  "extension": "keep-top",
  "profiles": {
    "target": {"api_key": "opaque-target", "target_extension": "remove"},
    "remaining": {"api_key": "opaque-remaining", "remaining_extension": "keep"}
  }
}
`)
	result := runLocalCommand(t, state, nil, "profile", "delete", "target", "--yes")
	if result.ExitCode != cli.ExitSuccess {
		t.Fatalf("result = %#v", result)
	}
	configData := string(readLocalFile(t, state.ConfigPath))
	authData := string(readLocalFile(t, state.AuthPath))
	for _, data := range []string{configData, authData} {
		if strings.Contains(data, "target_extension") || strings.Contains(data, `"target"`) || strings.Contains(data, "target:") {
			t.Fatalf("target-owned data remains:\n%s", data)
		}
		for _, want := range []string{"keep-top", "remaining_extension"} {
			if !strings.Contains(data, want) {
				t.Fatalf("unrelated unknown data %q missing:\n%s", want, data)
			}
		}
	}
}

func TestProfileMutationRefusalsDoNotCreateAbsentFiles(t *testing.T) {
	state := testutil.NewState(t)
	result := runLocalCommand(t, state, nil, "profile", "use", "missing")
	if result.ExitCode != cli.ExitUsage {
		t.Fatalf("result = %#v", result)
	}
	for _, path := range []string{state.ConfigPath, state.AuthPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s created: %v", path, err)
		}
	}
}
