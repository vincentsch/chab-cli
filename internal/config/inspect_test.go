package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/config"
)

func TestResolvePathsPrecedenceAndNoLoad(t *testing.T) {
	dir := t.TempDir()
	envConfig := filepath.Join(dir, "env.yml")
	envAuth := filepath.Join(dir, "env.json")
	flagConfig := filepath.Join(dir, "flag.yml")
	flagAuth := filepath.Join(dir, "flag.json")

	paths, err := config.ResolvePaths(config.Options{
		LookupEnv:     fakeEnv(nil),
		UserConfigDir: userConfigDir(dir),
	})
	if err != nil {
		t.Fatalf("ResolvePaths(default) error = %v", err)
	}
	if paths.ConfigPath != filepath.Join(dir, "chab", "config.yml") || paths.AuthPath != filepath.Join(dir, "chab", "auth.json") {
		t.Fatalf("default paths = %#v", paths)
	}
	if _, err := os.Stat(filepath.Join(dir, "chab")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ResolvePaths created state directory: %v", err)
	}

	paths, err = config.ResolvePaths(config.Options{
		LookupEnv: fakeEnv(map[string]string{
			"CHAB_CONFIG":    envConfig,
			"CHAB_AUTH_FILE": envAuth,
		}),
		UserConfigDir: userConfigDir(dir),
	})
	if err != nil {
		t.Fatalf("ResolvePaths(env) error = %v", err)
	}
	if paths.ConfigPath != envConfig || paths.AuthPath != envAuth {
		t.Fatalf("env paths = %#v", paths)
	}

	writeConfig(t, flagConfig, "version: [\n")
	paths, err = config.ResolvePaths(config.Options{
		Flags: config.FlagOverrides{
			ConfigPath: config.OverrideString{Value: flagConfig, Set: true},
			AuthPath:   config.OverrideString{Value: flagAuth, Set: true},
		},
		LookupEnv: fakeEnv(map[string]string{
			"CHAB_CONFIG":    envConfig,
			"CHAB_AUTH_FILE": envAuth,
		}),
		UserConfigDir: userConfigDir(dir),
	})
	if err != nil {
		t.Fatalf("ResolvePaths(flag malformed target) error = %v", err)
	}
	if paths.ConfigPath != flagConfig || paths.AuthPath != flagAuth {
		t.Fatalf("flag paths = %#v", paths)
	}
}

func TestConfigInspectionValuesAndKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	writeConfig(t, path, `
version: 1
current_profile: production.eu
profiles:
  local:
    base_url: https://local.example.test/root
    default_output: table
    defaults:
      project_list_limit: 12
  production.eu:
    base_url: https://prod.example.test
    api_base_url: https://api.prod.example.test/v1
    locale: de
    default_output: table
    defaults:
      project_list_limit: 50
`)

	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	view, persisted, err := config.ResolveProfileView(file, "local")
	if err != nil {
		t.Fatalf("ResolveProfileView(local) error = %v", err)
	}
	if !persisted || view.APIBaseURL != "https://local.example.test/root/v1" || view.Locale != "" || view.ProjectListLimit != 12 {
		t.Fatalf("local view = %#v persisted=%t", view, persisted)
	}
	view, persisted, err = config.ResolveProfileView(file, "missing")
	if err != nil {
		t.Fatalf("ResolveProfileView(missing) error = %v", err)
	}
	if persisted || view.BaseURL != config.DefaultBaseURL || view.APIBaseURL != config.DefaultAPIBaseURL {
		t.Fatalf("missing view = %#v persisted=%t", view, persisted)
	}

	listValues, err := config.ListValues(file)
	if err != nil {
		t.Fatalf("ListValues() error = %v", err)
	}
	values := valueMap(listValues)
	if values["current_profile"].Value != "production.eu" || values["current_profile"].Source != "file" {
		t.Fatalf("current_profile value = %#v", values["current_profile"])
	}
	if values["profiles.local.api_base_url"].Value != "https://local.example.test/root/v1" || values["profiles.local.api_base_url"].Source != "derived" {
		t.Fatalf("derived api value = %#v", values["profiles.local.api_base_url"])
	}
	if values["profiles.local.locale"].Value != nil || values["profiles.local.locale"].Source != "default" {
		t.Fatalf("unset locale value = %#v", values["profiles.local.locale"])
	}

	got, err := config.GetValue(file, "profiles.production.eu.api_base_url")
	if err != nil {
		t.Fatalf("GetValue(dot name) error = %v", err)
	}
	if got.Value != "https://api.prod.example.test/v1" || got.Source != "file" {
		t.Fatalf("dot-name value = %#v", got)
	}

	name, suffix, ok := config.ParseProfileKey("profiles.production.eu.defaults.project_list_limit")
	if !ok || name != "production.eu" || suffix != "defaults.project_list_limit" {
		t.Fatalf("ParseProfileKey() = %q %q %t", name, suffix, ok)
	}

	_, err = config.GetValue(file, "profiles.local.api_key")
	requireConfigKind(t, err, config.ErrUnsupportedKey, 1)
	_, err = config.GetValue(file, "profiles.missing.base_url")
	requireConfigKind(t, err, config.ErrMissingProfile, 1)
}

func TestConfigInspectionRejectsInvalidKnownValues(t *testing.T) {
	tests := []struct {
		name string
		body string
		key  string
		kind config.ErrorKind
	}{
		{
			name: "invalid base url",
			body: `
version: 1
current_profile: local
profiles:
  local:
    base_url: ftp://example.test
    default_output: table
    defaults:
      project_list_limit: 30
`,
			key:  "profiles.local.base_url",
			kind: config.ErrInvalidURL,
		},
		{
			name: "invalid api url",
			body: `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://file.example.test
    api_base_url: https://user@example.test
    default_output: table
    defaults:
      project_list_limit: 30
`,
			key:  "profiles.local.api_base_url",
			kind: config.ErrInvalidURL,
		},
		{
			name: "invalid locale",
			body: validConfigProfile("locale: fr"),
			key:  "profiles.local.locale",
			kind: config.ErrInvalidLocale,
		},
		{
			name: "invalid output",
			body: validConfigProfile("default_output: json"),
			key:  "profiles.local.default_output",
			kind: config.ErrInvalidDefault,
		},
		{
			name: "invalid list limit",
			body: validConfigProfile("defaults:\n      project_list_limit: 0"),
			key:  "profiles.local.defaults.project_list_limit",
			kind: config.ErrInvalidDefault,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			writeConfig(t, path, test.body)
			file, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			_, _, err = config.ResolveProfileView(file, "local")
			requireConfigKind(t, err, test.kind, 1)
			_, err = config.ListValues(file)
			requireConfigKind(t, err, test.kind, 1)
			_, err = config.GetValue(file, test.key)
			requireConfigKind(t, err, test.kind, 1)
		})
	}
}

func TestConfigGetRejectsAnInvalidDifferentProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	writeConfig(t, path, `
version: 1
current_profile: good
profiles:
  good:
    base_url: https://good.example.test
  bad:
    base_url: ftp://bad.example.test
`)
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	_, err = config.GetValue(file, "profiles.good.base_url")
	requireConfigKind(t, err, config.ErrInvalidURL, 1)
	_, err = config.GetValue(file, "current_profile")
	requireConfigKind(t, err, config.ErrInvalidURL, 1)
}

func TestValidateProfilesRejectsInvalidSiblingsWithoutInspectingSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	writeConfig(t, path, `
version: 1
current_profile: "Bad Name"
profiles:
  good:
    base_url: https://good.example.test
  bad:
    base_url: https://bad.example.test
    locale: fr
`)
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	err = config.ValidateProfiles(file)
	requireConfigKind(t, err, config.ErrInvalidLocale, 1)

	if _, _, err := config.SetProfileBaseURL(file, "bad", "https://bad.example.test"); err != nil {
		t.Fatalf("SetProfileBaseURL() error = %v", err)
	}
	if err := config.SetProfileLocale(file, "bad", "de"); err != nil {
		t.Fatalf("SetProfileLocale() error = %v", err)
	}
	if err := config.ValidateProfiles(file); err != nil {
		t.Fatalf("ValidateProfiles() rejected valid profiles with invalid stored selection: %v", err)
	}
}

func TestConfigInspectionRejectsInvalidProfileIdentifiers(t *testing.T) {
	invalidCurrent := &config.File{
		Version:        1,
		CurrentProfile: "Bad Name",
		Profiles:       map[string]config.Profile{},
	}
	_, err := config.CurrentProfileValue(invalidCurrent)
	requireConfigKind(t, err, config.ErrInvalidProfileName, 1)
	_, err = config.ListValues(invalidCurrent)
	requireConfigKind(t, err, config.ErrInvalidProfileName, 1)
	_, err = config.GetValue(invalidCurrent, "current_profile")
	requireConfigKind(t, err, config.ErrInvalidProfileName, 1)

	invalidPersisted := &config.File{
		Version:        1,
		CurrentProfile: "local",
		Profiles: map[string]config.Profile{
			"Bad Name": {BaseURL: "https://local.example.test"},
		},
	}
	_, err = config.ListValues(invalidPersisted)
	requireConfigKind(t, err, config.ErrInvalidProfileName, 1)
	_, _, err = config.ResolveProfileView(invalidPersisted, "Bad Name")
	requireConfigKind(t, err, config.ErrInvalidProfileName, 1)
}

func TestRuntimeOverrideBypassesInvalidPersistedSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	writeConfig(t, path, `
version: 1
current_profile: "Bad Name"
profiles:
  local:
    base_url: https://local.example.test
`)
	runtime, err := config.ResolveRuntime(config.Options{
		Flags: config.FlagOverrides{
			ConfigPath: config.OverrideString{Value: path, Set: true},
			Profile:    config.OverrideString{Value: "local", Set: true},
		},
		LookupEnv:     fakeEnv(nil),
		UserConfigDir: userConfigDir(t.TempDir()),
		Mode:          config.ResolveStrict,
	})
	if err != nil {
		t.Fatalf("ResolveRuntime() error = %v", err)
	}
	if runtime.Profile != "local" || runtime.BaseURL != "https://local.example.test" {
		t.Fatalf("runtime = %#v", runtime)
	}
}

func TestConfigSetProfileFieldWriters(t *testing.T) {
	t.Run("rederive omitted api", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yml")
		writeConfig(t, path, `
version: 1
current_profile: local
profiles:
  local:
    default_output: table
    defaults:
      project_list_limit: 30
`)
		file, err := config.Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		apiBase, rederived, err := config.SetProfileBaseURL(file, "local", "https://new.example.test/app")
		if err != nil {
			t.Fatalf("SetProfileBaseURL() error = %v", err)
		}
		if !rederived || apiBase != "https://new.example.test/app/v1" {
			t.Fatalf("rederived = %t api = %q", rederived, apiBase)
		}
		if err := config.Write(path, file); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		rt, err := config.ResolveRuntime(config.Options{
			Flags:     config.FlagOverrides{ConfigPath: config.OverrideString{Value: path, Set: true}},
			LookupEnv: fakeEnv(nil),
		})
		if err != nil {
			t.Fatalf("ResolveRuntime() error = %v", err)
		}
		if rt.BaseURL != "https://new.example.test/app" || rt.APIBaseURL != "https://new.example.test/app/v1" {
			t.Fatalf("runtime = %#v", rt)
		}
	})

	t.Run("preserve custom api and write other fields", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yml")
		writeConfig(t, path, `
version: 1
x_top: keep
current_profile: local
profiles:
  local:
    x_profile: keep
    base_url: https://old.example.test
    api_base_url: https://custom.example.test/v1
    locale: en
    default_output: table
    defaults:
      x_default: keep
      project_list_limit: 30
`)
		file, err := config.Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		apiBase, rederived, err := config.SetProfileBaseURL(file, "local", "https://new.example.test")
		if err != nil {
			t.Fatalf("SetProfileBaseURL() error = %v", err)
		}
		if rederived || apiBase != "" {
			t.Fatalf("custom api rederived = %t api = %q", rederived, apiBase)
		}
		if err := config.SetProfileAPIBaseURL(file, "local", "https://api.new.example.test/v1/"); err != nil {
			t.Fatalf("SetProfileAPIBaseURL() error = %v", err)
		}
		if err := config.SetProfileLocale(file, "local", ""); err != nil {
			t.Fatalf("SetProfileLocale() error = %v", err)
		}
		if err := config.SetProfileProjectListLimit(file, "local", "75"); err != nil {
			t.Fatalf("SetProfileProjectListLimit() error = %v", err)
		}
		if err := config.Write(path, file); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		data := string(readFile(t, path))
		for _, want := range []string{"x_top: keep", "x_profile: keep", "x_default: keep", "api_base_url: https://api.new.example.test/v1", "project_list_limit: 75"} {
			if !strings.Contains(data, want) {
				t.Fatalf("written config missing %q:\n%s", want, data)
			}
		}
	})

	t.Run("explicit old derived api stays derived", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yml")
		writeConfig(t, path, `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://old.example.test
    api_base_url: https://old.example.test/v1
    default_output: table
    defaults:
      project_list_limit: 30
`)
		file, err := config.Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		apiBase, rederived, err := config.SetProfileBaseURL(file, "local", "https://new.example.test")
		if err != nil {
			t.Fatalf("SetProfileBaseURL() error = %v", err)
		}
		if !rederived || apiBase != "https://new.example.test/v1" {
			t.Fatalf("rederived = %t api = %q", rederived, apiBase)
		}
		if err := config.Write(path, file); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		data := string(readFile(t, path))
		if !strings.Contains(data, "api_base_url: https://new.example.test/v1") {
			t.Fatalf("written config did not carry new derived api:\n%s", data)
		}
	})

	t.Run("validation", func(t *testing.T) {
		file := &config.File{}
		if err := config.UpsertProfile(file, "local", config.Profile{BaseURL: "https://example.test"}); err != nil {
			t.Fatalf("UpsertProfile() error = %v", err)
		}
		if err := config.SetProfileLocale(file, "local", "fr"); err == nil {
			t.Fatalf("SetProfileLocale(fr) succeeded")
		}
		if err := config.SetProfileProjectListLimit(file, "local", "0"); err == nil {
			t.Fatalf("SetProfileProjectListLimit(0) succeeded")
		}
		if _, _, err := config.SetProfileBaseURL(file, "missing", "https://example.test"); err == nil {
			t.Fatalf("SetProfileBaseURL(missing) succeeded")
		}
	})
}

func TestConfigDeleteProfileDisposition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	writeConfig(t, path, `
version: 1
current_profile: staging
profiles:
  local:
    base_url: https://local.example.test
    default_output: table
    defaults:
      project_list_limit: 30
  staging:
    base_url: https://staging.example.test
    default_output: table
    defaults:
      project_list_limit: 30
`)
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	res, err := config.DeleteProfile(file, "staging")
	if err != nil {
		t.Fatalf("DeleteProfile(current) error = %v", err)
	}
	if !res.Existed || !res.WasCurrent || !res.CurrentChanged || res.NewCurrent != "local" || res.Disposition != config.DispositionWrite {
		t.Fatalf("delete current result = %#v", res)
	}

	writeConfig(t, path, `
version: 1
current_profile: beta
profiles:
  beta:
    base_url: https://beta.example.test
    default_output: table
    defaults:
      project_list_limit: 30
  gamma:
    base_url: https://gamma.example.test
    default_output: table
    defaults:
      project_list_limit: 30
`)
	file, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load(non-current) error = %v", err)
	}
	res, err = config.DeleteProfile(file, "gamma")
	if err != nil {
		t.Fatalf("DeleteProfile(non-current) error = %v", err)
	}
	if !res.Existed || res.WasCurrent || res.CurrentChanged || file.CurrentProfile != "beta" {
		t.Fatalf("delete non-current result = %#v current=%q", res, file.CurrentProfile)
	}

	writeConfig(t, path, `
version: 1
current_profile: missing
profiles:
  beta:
    base_url: https://beta.example.test
    default_output: table
    defaults:
      project_list_limit: 30
  gamma:
    base_url: https://gamma.example.test
    default_output: table
    defaults:
      project_list_limit: 30
`)
	file, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load(missing current) error = %v", err)
	}
	res, err = config.DeleteProfile(file, "gamma")
	if err != nil {
		t.Fatalf("DeleteProfile(repair) error = %v", err)
	}
	if !res.PreviousCurrentMissing || res.NewCurrent != "beta" || file.CurrentProfile != "beta" {
		t.Fatalf("repair result = %#v current=%q", res, file.CurrentProfile)
	}

	writeConfig(t, path, `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://local.example.test
    default_output: table
    defaults:
      project_list_limit: 30
`)
	file, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load(final) error = %v", err)
	}
	res, err = config.DeleteProfile(file, "local")
	if err != nil {
		t.Fatalf("DeleteProfile(final) error = %v", err)
	}
	if res.Disposition != config.DispositionRemoveFile || res.RemainingCount != 0 {
		t.Fatalf("final owned result = %#v", res)
	}

	writeConfig(t, path, `
version: 1
x_top: keep
current_profile: local
profiles:
  local:
    base_url: https://local.example.test
    default_output: table
    defaults:
      project_list_limit: 30
`)
	file, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load(unknown) error = %v", err)
	}
	res, err = config.DeleteProfile(file, "local")
	if err != nil {
		t.Fatalf("DeleteProfile(unknown) error = %v", err)
	}
	if res.Disposition != config.DispositionKeepFinalProfile {
		t.Fatalf("unknown-key final result = %#v", res)
	}

	writeConfig(t, path, `
version: 1
# keep
current_profile: local
profiles:
  local:
    base_url: https://local.example.test
    default_output: table
    defaults:
      project_list_limit: 30
`)
	file, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load(comment) error = %v", err)
	}
	res, err = config.DeleteProfile(file, "local")
	if err != nil {
		t.Fatalf("DeleteProfile(comment) error = %v", err)
	}
	if res.Disposition != config.DispositionKeepFinalProfile {
		t.Fatalf("comment final result = %#v", res)
	}

	for _, test := range []struct {
		name string
		body string
	}{
		{
			name: "profile field head comment",
			body: `
version: 1
current_profile: local
profiles:
  local:
    # keep field context
    base_url: https://local.example.test
`,
		},
		{
			name: "profile field line comment",
			body: `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://local.example.test # keep inline context
`,
		},
		{
			name: "nested defaults comment",
			body: `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://local.example.test
    defaults:
      # keep limit context
      project_list_limit: 30
`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			writeConfig(t, path, test.body)
			file, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			res, err := config.DeleteProfile(file, "local")
			if err != nil {
				t.Fatalf("DeleteProfile() error = %v", err)
			}
			if res.Disposition != config.DispositionKeepFinalProfile {
				t.Fatalf("nested-comment final result = %#v", res)
			}
		})
	}

	if _, err := config.DeleteProfile(file, "bad name"); err == nil {
		t.Fatalf("DeleteProfile(invalid name) succeeded")
	}
}

func valueMap(values []config.ConfigValue) map[string]config.ConfigValue {
	out := map[string]config.ConfigValue{}
	for _, value := range values {
		out[value.Key] = value
	}
	return out
}
