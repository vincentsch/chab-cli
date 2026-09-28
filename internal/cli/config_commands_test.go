package cli_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestConfigPathIsReadOnlyAndSupportsEveryInspectionMode(t *testing.T) {
	state := testutil.NewState(t)
	testutil.WriteMalformedConfig(t, state.ConfigPath)
	testutil.WriteMalformedAuth(t, state.AuthPath)
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "human", args: []string{"config", "path"}},
		{name: "plain", args: []string{"config", "path", "--plain"}},
		{name: "json", args: []string{"config", "path", "--json"}},
		{name: "jq", args: []string{"config", "path", "--jq", ".auth_path"}},
		{name: "template", args: []string{"config", "path", "--template", "{{.config_path}}"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			beforeConfig := readLocalFile(t, state.ConfigPath)
			beforeAuth := readLocalFile(t, state.AuthPath)
			result := runLocalCommand(t, state, nil, test.args...)
			if result.ExitCode != cli.ExitSuccess || result.Stderr != "" {
				t.Fatalf("result = %#v", result)
			}
			switch test.name {
			case "json":
				var paths struct {
					ConfigPath string `json:"config_path"`
					AuthPath   string `json:"auth_path"`
				}
				if err := json.Unmarshal([]byte(result.Stdout), &paths); err != nil {
					t.Fatalf("json output did not parse: %v\n%s", err, result.Stdout)
				}
				if paths.ConfigPath != state.ConfigPath || paths.AuthPath != state.AuthPath {
					t.Fatalf("json paths = %#v, want config=%q auth=%q", paths, state.ConfigPath, state.AuthPath)
				}
			case "jq":
				var authPath string
				if err := json.Unmarshal([]byte(result.Stdout), &authPath); err != nil {
					t.Fatalf("jq output did not parse: %v\n%s", err, result.Stdout)
				}
				if authPath != state.AuthPath {
					t.Fatalf("jq auth path = %q, want %q", authPath, state.AuthPath)
				}
			case "template":
				if !strings.Contains(result.Stdout, state.ConfigPath) {
					t.Fatalf("stdout missing config path: %q", result.Stdout)
				}
			default:
				if !strings.Contains(result.Stdout, state.ConfigPath) {
					t.Fatalf("stdout missing config path: %q", result.Stdout)
				}
				if !strings.Contains(result.Stdout, state.AuthPath) {
					t.Fatalf("stdout missing auth path: %q", result.Stdout)
				}
			}
			if !reflect.DeepEqual(readLocalFile(t, state.ConfigPath), beforeConfig) ||
				!reflect.DeepEqual(readLocalFile(t, state.AuthPath), beforeAuth) {
				t.Fatal("config path changed local files")
			}
		})
	}

	envState := testutil.NewState(t)
	result := testutil.RunCommandWith(t, testutil.Options{
		LookupEnv: testutil.HermeticEnv(map[string]string{
			"CHAB_CONFIG":    envState.ConfigPath,
			"CHAB_AUTH_FILE": envState.AuthPath,
		}),
	}, "config", "path", "--json")
	var envPaths struct {
		ConfigPath string `json:"config_path"`
		AuthPath   string `json:"auth_path"`
	}
	envJSONErr := json.Unmarshal([]byte(result.Stdout), &envPaths)
	if result.ExitCode != cli.ExitSuccess || envJSONErr != nil || envPaths.ConfigPath != envState.ConfigPath || envPaths.AuthPath != envState.AuthPath {
		t.Fatalf("environment path result = %#v", result)
	}
	assertLocalFilesAbsent(t, envState)
}

func TestConfigInspectionTypesSourcesOrderingAndUnknownFiltering(t *testing.T) {
	state := testutil.NewState(t)
	noFiles := runLocalCommand(t, state, nil, "config", "list", "--json")
	if noFiles.ExitCode != cli.ExitSuccess || noFiles.Stderr != "" {
		t.Fatalf("no-file list = %#v", noFiles)
	}
	var result struct {
		ConfigPath string `json:"config_path"`
		Values     []struct {
			Key    string `json:"key"`
			Value  any    `json:"value"`
			Source string `json:"source"`
		} `json:"values"`
	}
	if err := json.Unmarshal([]byte(noFiles.Stdout), &result); err != nil {
		t.Fatal(err)
	}
	if result.ConfigPath != state.ConfigPath {
		t.Fatalf("config path = %q", result.ConfigPath)
	}
	var keys []string
	values := map[string]struct {
		Value  any
		Source string
	}{}
	for _, value := range result.Values {
		keys = append(keys, value.Key)
		values[value.Key] = struct {
			Value  any
			Source string
		}{value.Value, value.Source}
	}
	if !sortStringsEqual(keys, append([]string(nil), keys...)) {
		t.Fatalf("values are not sorted: %v", keys)
	}
	if values["profiles.local.locale"].Value != nil || values["profiles.local.locale"].Source != "default" {
		t.Fatalf("locale value = %#v", values["profiles.local.locale"])
	}
	if _, ok := values["profiles.local.defaults.project_list_limit"].Value.(float64); !ok {
		t.Fatalf("project limit type = %T", values["profiles.local.defaults.project_list_limit"].Value)
	}
	if values["profiles.local.api_base_url"].Source != "derived" {
		t.Fatalf("api base source = %#v", values["profiles.local.api_base_url"])
	}
	assertLocalFilesAbsent(t, state)

	writeLocalFixture(t, state.ConfigPath, `version: 1
current_profile: team.eu
unknown_top: keep
profiles:
  team.eu:
    base_url: https://team.example.test/root/
    locale: ""
    extension: hidden
    defaults:
      project_list_limit: 41
`)
	list := runLocalCommand(t, state, nil, "config", "list", "--json")
	if list.ExitCode != cli.ExitSuccess || strings.Contains(list.Stdout, "unknown_top") || strings.Contains(list.Stdout, "extension") {
		t.Fatalf("filtered list result = %#v", list)
	}
	var persisted struct {
		Values []struct {
			Key    string `json:"key"`
			Value  any    `json:"value"`
			Source string `json:"source"`
		} `json:"values"`
	}
	if err := json.Unmarshal([]byte(list.Stdout), &persisted); err != nil {
		t.Fatal(err)
	}
	persistedValues := map[string]struct {
		Value  any
		Source string
	}{}
	for _, value := range persisted.Values {
		persistedValues[value.Key] = struct {
			Value  any
			Source string
		}{value.Value, value.Source}
	}
	base := persistedValues["profiles.team.eu.base_url"]
	if _, ok := base.Value.(string); !ok || base.Source != "file" {
		t.Fatalf("persisted base URL value = %#v", base)
	}
	if derived := persistedValues["profiles.team.eu.api_base_url"]; derived.Source != "derived" {
		t.Fatalf("derived API base value = %#v", derived)
	}
	for _, mode := range [][]string{
		{"config", "list"},
		{"config", "list", "--plain"},
		{"config", "list", "--jq", ".values[0].key"},
		{"config", "list", "--template", "{{range .values}}{{.key}}{{end}}"},
		{"config", "get", "profiles.team.eu.base_url"},
		{"config", "get", "profiles.team.eu.base_url", "--plain"},
		{"config", "get", "profiles.team.eu.base_url", "--json"},
		{"config", "get", "profiles.team.eu.base_url", "--jq", ".value"},
		{"config", "get", "profiles.team.eu.base_url", "--template", "{{.value}}"},
	} {
		output := runLocalCommand(t, state, nil, mode...)
		if output.ExitCode != cli.ExitSuccess {
			t.Fatalf("%v result = %#v", mode, output)
		}
	}
	plainList := runLocalCommand(t, state, nil, "config", "list", "--plain")
	if plainList.ExitCode != cli.ExitSuccess || !strings.Contains(plainList.Stderr, state.ConfigPath) ||
		strings.Contains(plainList.Stdout, state.ConfigPath) {
		t.Fatalf("plain list output boundary = %#v", plainList)
	}
	plainGet := runLocalCommand(t, state, nil, "config", "get", "profiles.team.eu.locale", "--plain")
	if plainGet.ExitCode != cli.ExitSuccess || plainGet.Stderr != "" ||
		!strings.Contains(plainGet.Stdout, "value\t") {
		t.Fatalf("plain empty-locale output = %#v", plainGet)
	}

	set := runLocalCommand(t, state, nil, "config", "set", "profiles.team.eu.locale", "de")
	if set.ExitCode != cli.ExitSuccess {
		t.Fatalf("config write for source transition = %#v", set)
	}
	materialized := runLocalCommand(t, state, nil, "config", "get", "profiles.team.eu.api_base_url", "--json")
	if materialized.ExitCode != cli.ExitSuccess || !strings.Contains(materialized.Stdout, `"source": "file"`) {
		t.Fatalf("materialized API base source = %#v", materialized)
	}

	missing := runLocalCommand(t, state, nil, "config", "get", "profiles.missing.locale")
	if missing.ExitCode != cli.ExitUsage || missing.Stdout != "" {
		t.Fatalf("missing profile get = %#v", missing)
	}

	if result := runLocalCommand(t, testutil.NewState(t), nil, "config", "get", "current_profile", "--json"); result.ExitCode != cli.ExitSuccess || !strings.Contains(result.Stdout, `"value": "local"`) || !strings.Contains(result.Stdout, `"source": "default"`) {
		t.Fatalf("default current profile = %#v", result)
	}
}

func TestConfigSetWritableKeysAndRefusals(t *testing.T) {
	state := testutil.NewState(t)
	testutil.WriteConfigProfile(t, state.ConfigPath, "one.eu", config.Profile{BaseURL: "https://one.example.test"}, true)
	testutil.WriteConfigProfile(t, state.ConfigPath, "two", config.Profile{BaseURL: "https://two.example.test"}, false)

	for _, args := range [][]string{
		{"config", "set", "current_profile", "two"},
		{"config", "set", "profiles.one.eu.base_url", "https://changed.example.test/root/"},
		{"config", "set", "profiles.one.eu.api_base_url", "https://api.changed.example.test/v1/"},
		{"config", "set", "profiles.one.eu.locale", ""},
		{"config", "set", "profiles.one.eu.defaults.project_list_limit", "77"},
	} {
		result := runLocalCommand(t, state, nil, args...)
		if result.ExitCode != cli.ExitSuccess || result.Err != nil {
			t.Fatalf("%v result = %#v", args, result)
		}
	}
	file, err := config.Load(state.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if file.CurrentProfile != "two" {
		t.Fatalf("current profile = %q", file.CurrentProfile)
	}
	view, _, err := config.ResolveProfileView(file, "one.eu")
	if err != nil {
		t.Fatal(err)
	}
	if view.BaseURL != "https://changed.example.test/root" ||
		view.APIBaseURL != "https://api.changed.example.test/v1" ||
		view.Locale != "" || view.ProjectListLimit != 77 {
		t.Fatalf("updated view = %#v", view)
	}

	for _, test := range []struct {
		name string
		key  string
		val  string
	}{
		{name: "missing current", key: "current_profile", val: "missing"},
		{name: "missing profile", key: "profiles.missing.locale", val: "de"},
		{name: "unsupported", key: "unknown.key", val: "value"},
		{name: "default output", key: "profiles.one.eu.default_output", val: "json"},
		{name: "secret", key: "profiles.one.eu.api_key", val: "secret"},
		{name: "invalid locale", key: "profiles.one.eu.locale", val: "fr"},
		{name: "invalid limit", key: "profiles.one.eu.defaults.project_list_limit", val: "0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := readLocalFile(t, state.ConfigPath)
			result := runLocalCommand(t, state, nil, "config", "set", test.key, test.val)
			if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
				t.Fatalf("result = %#v", result)
			}
			if !reflect.DeepEqual(readLocalFile(t, state.ConfigPath), before) {
				t.Fatal("refused config set changed config bytes")
			}
		})
	}

	t.Run("config-only keys ignore malformed auth", func(t *testing.T) {
		local := testutil.NewState(t)
		testutil.WriteConfigProfile(t, local.ConfigPath, "one", config.Profile{BaseURL: "https://one.example.test"}, true)
		testutil.WriteConfigProfile(t, local.ConfigPath, "two", config.Profile{BaseURL: "https://two.example.test"}, false)
		testutil.WriteMalformedAuth(t, local.AuthPath)
		beforeAuth := readLocalFile(t, local.AuthPath)
		for _, args := range [][]string{
			{"config", "set", "profiles.one.locale", "de"},
			{"config", "set", "current_profile", "two"},
		} {
			result := runLocalCommand(t, local, nil, args...)
			if result.ExitCode != cli.ExitSuccess {
				t.Fatalf("%v result = %#v", args, result)
			}
			if !reflect.DeepEqual(readLocalFile(t, local.AuthPath), beforeAuth) {
				t.Fatalf("%v changed auth bytes", args)
			}
		}
	})

	preserved := testutil.NewState(t)
	writeLocalFixture(t, preserved.ConfigPath, `# keep comment
version: 1
current_profile: local
extension: keep-top
profiles:
  local:
    base_url: https://local.example.test
    profile_extension: keep-profile
`)
	result := runLocalCommand(t, preserved, nil, "config", "set", "profiles.local.locale", "de")
	if result.ExitCode != cli.ExitSuccess {
		t.Fatalf("preservation set = %#v", result)
	}
	data := string(readLocalFile(t, preserved.ConfigPath))
	for _, want := range []string{"# keep comment", "extension: keep-top", "profile_extension: keep-profile"} {
		if !strings.Contains(data, want) {
			t.Fatalf("config set dropped %q:\n%s", want, data)
		}
	}
}

func TestConfigSetProtectsStoredCredentialDestination(t *testing.T) {
	newState := func(t *testing.T, customAPI bool, withAuth bool) testutil.State {
		t.Helper()
		state := testutil.NewState(t)
		profile := config.Profile{BaseURL: "https://old.example.test"}
		if customAPI {
			profile.APIBaseURL = "https://custom-api.example.test/v1"
		}
		testutil.WriteConfigProfile(t, state.ConfigPath, "local", profile, true)
		if withAuth {
			testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(testutil.FakeKey("destination_guard")))
		}
		return state
	}

	t.Run("no auth allows destination change", func(t *testing.T) {
		state := newState(t, false, false)
		result := runLocalCommand(t, state, nil, "config", "set", "profiles.local.base_url", "https://new.example.test")
		if result.ExitCode != cli.ExitSuccess || !strings.Contains(result.Stdout, "Re-derived api_base_url") {
			t.Fatalf("result = %#v", result)
		}
		assertNotExists(t, state.AuthPath)
	})

	t.Run("missing config target without auth", func(t *testing.T) {
		state := testutil.NewState(t)
		result := runLocalCommand(t, state, nil, "config", "set", "profiles.local.api_base_url", "https://new.example.test/v1")
		if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
			t.Fatalf("result = %#v", result)
		}
		assertLocalFilesAbsent(t, state)
	})

	t.Run("normalized no-op with auth", func(t *testing.T) {
		state := newState(t, false, true)
		beforeAuth := readLocalFile(t, state.AuthPath)
		result := runLocalCommand(t, state, nil, "config", "set", "profiles.local.api_base_url", "https://old.example.test/v1/")
		if result.ExitCode != cli.ExitSuccess {
			t.Fatalf("result = %#v", result)
		}
		if !reflect.DeepEqual(readLocalFile(t, state.AuthPath), beforeAuth) {
			t.Fatal("config set rewrote auth")
		}
	})

	t.Run("equivalent base URL with auth", func(t *testing.T) {
		state := newState(t, false, true)
		beforeAuth := readLocalFile(t, state.AuthPath)
		result := runLocalCommand(t, state, nil, "config", "set", "profiles.local.base_url", "https://old.example.test/")
		if result.ExitCode != cli.ExitSuccess {
			t.Fatalf("result = %#v", result)
		}
		if !reflect.DeepEqual(readLocalFile(t, state.AuthPath), beforeAuth) {
			t.Fatal("config set rewrote auth")
		}
	})

	t.Run("auth-only target is not a config profile", func(t *testing.T) {
		state := testutil.NewState(t)
		testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(testutil.FakeKey("auth_only_destination")))
		beforeAuth := readLocalFile(t, state.AuthPath)
		result := runLocalCommand(t, state, nil, "config", "set", "profiles.local.api_base_url", "https://new.example.test/v1")
		if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
			t.Fatalf("result = %#v", result)
		}
		assertNotExists(t, state.ConfigPath)
		if !reflect.DeepEqual(readLocalFile(t, state.AuthPath), beforeAuth) {
			t.Fatal("missing-config refusal changed auth")
		}
	})

	for _, test := range []struct {
		name string
		key  string
		val  string
	}{
		{name: "derived destination via base", key: "profiles.local.base_url", val: "https://new.example.test"},
		{name: "explicit API destination", key: "profiles.local.api_base_url", val: "https://api.new.example.test/v1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := newState(t, false, true)
			beforeConfig := readLocalFile(t, state.ConfigPath)
			beforeAuth := readLocalFile(t, state.AuthPath)
			result := runLocalCommand(t, state, nil, "config", "set", test.key, test.val)
			if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
				t.Fatalf("result = %#v", result)
			}
			for _, want := range []string{"chab logout --profile local", "update the URL", "chab login --profile local"} {
				if !strings.Contains(result.Stderr, want) {
					t.Fatalf("stderr missing %q: %s", want, result.Stderr)
				}
			}
			if !reflect.DeepEqual(readLocalFile(t, state.ConfigPath), beforeConfig) ||
				!reflect.DeepEqual(readLocalFile(t, state.AuthPath), beforeAuth) {
				t.Fatal("destination refusal changed local files")
			}
		})
	}

	t.Run("custom API remains unchanged", func(t *testing.T) {
		state := newState(t, true, true)
		beforeAuth := readLocalFile(t, state.AuthPath)
		result := runLocalCommand(t, state, nil, "config", "set", "profiles.local.base_url", "https://new.example.test")
		if result.ExitCode != cli.ExitSuccess {
			t.Fatalf("result = %#v", result)
		}
		file, err := config.Load(state.ConfigPath)
		if err != nil {
			t.Fatal(err)
		}
		view, _, err := config.ResolveProfileView(file, "local")
		if err != nil {
			t.Fatal(err)
		}
		if view.APIBaseURL != "https://custom-api.example.test/v1" {
			t.Fatalf("API base = %q", view.APIBaseURL)
		}
		if !reflect.DeepEqual(readLocalFile(t, state.AuthPath), beforeAuth) {
			t.Fatal("auth bytes changed")
		}
	})

	t.Run("invalid API URL repairs without auth", func(t *testing.T) {
		state := testutil.NewState(t)
		writeLocalFixture(t, state.ConfigPath, "version: 1\ncurrent_profile: local\nprofiles:\n  local:\n    base_url: https://old.example.test\n    api_base_url: ftp://invalid.example\n")
		result := runLocalCommand(t, state, nil, "config", "set", "profiles.local.api_base_url", "https://repaired.example.test/v1")
		if result.ExitCode != cli.ExitSuccess {
			t.Fatalf("result = %#v", result)
		}
		show := runLocalCommand(t, state, nil, "profile", "show", "local", "--json")
		if show.ExitCode != cli.ExitSuccess || !strings.Contains(show.Stdout, "https://repaired.example.test/v1") {
			t.Fatalf("repaired show = %#v", show)
		}
	})

	t.Run("invalid API URL fails closed with auth", func(t *testing.T) {
		state := testutil.NewState(t)
		writeLocalFixture(t, state.ConfigPath, "version: 1\ncurrent_profile: local\nprofiles:\n  local:\n    base_url: https://old.example.test\n    api_base_url: ftp://invalid.example\n")
		testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(testutil.FakeKey("unresolved_destination")))
		beforeConfig := readLocalFile(t, state.ConfigPath)
		beforeAuth := readLocalFile(t, state.AuthPath)
		result := runLocalCommand(t, state, nil, "config", "set", "profiles.local.api_base_url", "https://repaired.example.test/v1")
		if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
			t.Fatalf("result = %#v", result)
		}
		if !reflect.DeepEqual(readLocalFile(t, state.ConfigPath), beforeConfig) ||
			!reflect.DeepEqual(readLocalFile(t, state.AuthPath), beforeAuth) {
			t.Fatal("fail-closed set changed local files")
		}
	})

	t.Run("malformed auth precedes URL write", func(t *testing.T) {
		state := newState(t, false, false)
		testutil.WriteMalformedAuth(t, state.AuthPath)
		beforeConfig := readLocalFile(t, state.ConfigPath)
		beforeAuth := readLocalFile(t, state.AuthPath)
		result := runLocalCommand(t, state, nil, "config", "set", "profiles.local.api_base_url", "https://new.example.test/v1")
		if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
			t.Fatalf("result = %#v", result)
		}
		if !reflect.DeepEqual(readLocalFile(t, state.ConfigPath), beforeConfig) ||
			!reflect.DeepEqual(readLocalFile(t, state.AuthPath), beforeAuth) {
			t.Fatal("malformed-auth refusal changed files")
		}
	})
}

func TestConfigListRejectsInvalidProfileAsAUnit(t *testing.T) {
	t.Run("invalid known profile value", func(t *testing.T) {
		state := testutil.NewState(t)
		writeLocalFixture(t, state.ConfigPath, `version: 1
current_profile: good
profiles:
  good:
    base_url: https://good.example.test
  bad:
    base_url: https://bad.example.test
    locale: fr
`)
		for _, args := range [][]string{
			{"config", "list", "--json"},
			{"config", "get", "profiles.bad.base_url", "--json"},
			{"config", "get", "profiles.good.base_url", "--json"},
			{"config", "get", "current_profile", "--json"},
			{"profile", "show", "good", "--json"},
		} {
			result := runLocalCommand(t, state, nil, args...)
			if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
				t.Fatalf("%v result = %#v", args, result)
			}
		}
	})

	for _, test := range []struct {
		name     string
		body     string
		commands [][]string
	}{
		{
			name: "invalid current profile",
			body: `version: 1
current_profile: "Bad Name"
profiles:
  local:
    base_url: https://local.example.test
`,
			commands: [][]string{
				{"config", "get", "current_profile", "--json"},
				{"config", "list", "--json"},
				{"profile", "list", "--json"},
			},
		},
		{
			name: "invalid persisted profile name",
			body: `version: 1
current_profile: local
profiles:
  "Bad Name":
    base_url: https://local.example.test
`,
			commands: [][]string{
				{"config", "get", "profiles.Bad Name.base_url", "--json"},
				{"config", "list", "--json"},
				{"profile", "list", "--json"},
				{"profile", "show", "local", "--json"},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := testutil.NewState(t)
			writeLocalFixture(t, state.ConfigPath, test.body)
			for _, args := range test.commands {
				result := runLocalCommand(t, state, nil, args...)
				if result.ExitCode != cli.ExitUsage || result.Stdout != "" ||
					!strings.Contains(result.Stderr, "invalid_profile_name") {
					t.Fatalf("%v result = %#v", args, result)
				}
			}
		})
	}
}

func TestProfileAndConfigCommandsRejectAmbiguousYAML(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{
			name: "duplicate known field",
			body: `version: 1
current_profile: local
profiles:
  local:
    base_url: https://one.example.test
    base_url: https://two.example.test
`,
		},
		{
			name: "additional document",
			body: `version: 1
current_profile: local
profiles:
  local:
    base_url: https://local.example.test
---
preserved_marker: must-not-disappear
`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := testutil.NewState(t)
			writeLocalFixture(t, state.ConfigPath, test.body)
			before := readLocalFile(t, state.ConfigPath)
			for _, args := range [][]string{
				{"profile", "list", "--json"},
				{"config", "list", "--json"},
				{"config", "get", "current_profile", "--json"},
				{"config", "set", "current_profile", "local"},
			} {
				result := runLocalCommand(t, state, nil, args...)
				if result.ExitCode != cli.ExitUsage || result.Stdout != "" ||
					!strings.Contains(result.Stderr, "malformed_config") {
					t.Fatalf("%v result = %#v", args, result)
				}
				if !reflect.DeepEqual(readLocalFile(t, state.ConfigPath), before) {
					t.Fatalf("%v changed ambiguous config", args)
				}
			}
		})
	}
}

func TestInvalidCurrentProfileCanBeRepairedOrOverridden(t *testing.T) {
	const invalidCurrent = `version: 1
current_profile: "Bad Name"
profiles:
  local:
    base_url: https://local.example.test
`

	t.Run("config set", func(t *testing.T) {
		state := testutil.NewState(t)
		writeLocalFixture(t, state.ConfigPath, invalidCurrent)
		result := runLocalCommand(t, state, nil, "config", "set", "current_profile", "local")
		if result.ExitCode != cli.ExitSuccess {
			t.Fatalf("result = %#v", result)
		}
		file, err := config.Load(state.ConfigPath)
		if err != nil {
			t.Fatal(err)
		}
		if file.CurrentProfile != "local" {
			t.Fatalf("current profile = %q", file.CurrentProfile)
		}
	})

	t.Run("profile use", func(t *testing.T) {
		state := testutil.NewState(t)
		writeLocalFixture(t, state.ConfigPath, invalidCurrent)
		result := runLocalCommand(t, state, nil, "profile", "use", "local")
		if result.ExitCode != cli.ExitSuccess {
			t.Fatalf("result = %#v", result)
		}
		file, err := config.Load(state.ConfigPath)
		if err != nil {
			t.Fatal(err)
		}
		if file.CurrentProfile != "local" {
			t.Fatalf("current profile = %q", file.CurrentProfile)
		}
	})

	t.Run("explicit profile override", func(t *testing.T) {
		state := testutil.NewState(t)
		writeLocalFixture(t, state.ConfigPath, invalidCurrent)
		for _, args := range [][]string{
			{"--profile", "local", "profile", "list", "--json"},
			{"--profile", "local", "profile", "show", "--json"},
		} {
			result := runLocalCommand(t, state, nil, args...)
			if result.ExitCode != cli.ExitSuccess ||
				!strings.Contains(result.Stdout, `"current_profile": "local"`) &&
					!strings.Contains(result.Stdout, `"name": "local"`) {
				t.Fatalf("%v result = %#v", args, result)
			}
		}
	})
}

func sortStringsEqual(left, right []string) bool {
	sorted := append([]string(nil), right...)
	sort.Strings(sorted)
	return reflect.DeepEqual(left, sorted)
}
