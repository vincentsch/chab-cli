package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/config"
)

func TestResolveRuntimeDefaultsAndNoCreation(t *testing.T) {
	dir := t.TempDir()

	for _, mode := range []config.ResolveMode{config.ResolveStrict, config.ResolveForWrite} {
		t.Run(modeName(mode), func(t *testing.T) {
			rt, err := config.ResolveRuntime(config.Options{
				LookupEnv:     fakeEnv(nil),
				UserConfigDir: userConfigDir(dir),
				Mode:          mode,
			})
			if err != nil {
				t.Fatalf("ResolveRuntime() error = %v", err)
			}
			if rt.Profile != "local" || rt.BaseURL != config.DefaultBaseURL || rt.APIBaseURL != config.DefaultAPIBaseURL {
				t.Fatalf("runtime URL defaults = %#v", rt)
			}
			if rt.Locale != "" || rt.DefaultOutput != "table" || rt.ProjectListLimit != 30 {
				t.Fatalf("runtime carried defaults = %#v", rt)
			}
			if rt.ConfigPath != filepath.Join(dir, "chab", "config.yml") {
				t.Fatalf("ConfigPath = %q", rt.ConfigPath)
			}
			if rt.AuthPath != filepath.Join(dir, "chab", "auth.json") {
				t.Fatalf("AuthPath = %q", rt.AuthPath)
			}
			if _, err := os.Stat(filepath.Join(dir, "chab")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("ResolveRuntime created config directory, stat err = %v", err)
			}
		})
	}
}

func TestProductionDefaultPairDoesNotCaptureCustomOrSavedHosts(t *testing.T) {
	for _, tc := range []struct {
		base string
		want string
	}{
		{"https://www.chab.ai", "https://www.chab.ai/v1"},
		{"https://www.chab.ai/", "https://www.chab.ai/v1"},
		{"https://app.chab.ai", "https://api.chab.ai/v1"},
		{"https://www.chab.ai/staging", "https://www.chab.ai/staging/v1"},
		{"https://staging.chab.ai", "https://staging.chab.ai/v1"},
		{"http://localhost", "http://localhost/v1"},
	} {
		if got := config.DeriveAPIBaseURL(tc.base); got != tc.want {
			t.Fatalf("DeriveAPIBaseURL(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}
	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, "chab", "config.yml"), `version: 1
current_profile: local
profiles:
  local:
    base_url: http://localhost
    api_base_url: http://localhost/v1
`)
	rt, err := config.ResolveRuntime(config.Options{LookupEnv: fakeEnv(nil), UserConfigDir: userConfigDir(dir), Mode: config.ResolveStrict})
	if err != nil || rt.BaseURL != "http://localhost" || rt.APIBaseURL != "http://localhost/v1" || rt.BaseURLSource != config.RuntimeValueSourceFile || rt.APIBaseURLSource != config.RuntimeValueSourceFile {
		t.Fatalf("persisted local profile changed by defaults: runtime=%#v, err=%v", rt, err)
	}
}

func TestSavedLegacyAppOriginKeepsItsAPIHostWithCredential(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, "chab", "config.yml"), `version: 1
current_profile: local
profiles:
  local:
    base_url: https://app.chab.ai
`)
	authPath := filepath.Join(dir, "chab", "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"version":1,"profiles":{"local":{"api_key":"ak_legacy|secret"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	rt, err := config.ResolveRuntime(config.Options{LookupEnv: fakeEnv(nil), UserConfigDir: userConfigDir(dir), Mode: config.ResolveStrict})
	if err != nil {
		t.Fatal(err)
	}
	if rt.BaseURL != "https://app.chab.ai" || rt.APIBaseURL != "https://api.chab.ai/v1" {
		t.Fatalf("saved legacy destination drift: app=%q API=%q", rt.BaseURL, rt.APIBaseURL)
	}
}

func TestLegacyImplicitLocalDestinationIsNotRedirected(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configYAML string
		authOnly   bool
	}{
		{name: "persisted URL-omitting profile", configYAML: "version: 1\ncurrent_profile: local\nprofiles:\n  local:\n    locale: en\n"},
		{name: "auth-only legacy installation", authOnly: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.configYAML != "" {
				writeConfig(t, filepath.Join(dir, "chab", "config.yml"), tc.configYAML)
			}
			if tc.authOnly {
				path := filepath.Join(dir, "chab", "auth.json")
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(`{"version":1,"profiles":{"local":{"api_key":"ak_legacy|secret"}}}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			rt, err := config.ResolveRuntime(config.Options{LookupEnv: fakeEnv(nil), UserConfigDir: userConfigDir(dir), Mode: config.ResolveStrict})
			if err != nil {
				t.Fatal(err)
			}
			if rt.BaseURL != config.LegacyImplicitBaseURL || rt.APIBaseURL != "http://localhost/v1" {
				t.Fatalf("legacy destination changed: %#v", rt)
			}
			if tc.configYAML != "" {
				file, err := config.Load(rt.ConfigPath)
				if err != nil {
					t.Fatal(err)
				}
				view, _, err := config.ResolveProfileView(file, "local")
				if err != nil || view.BaseURL != rt.BaseURL || view.APIBaseURL != rt.APIBaseURL {
					t.Fatalf("inspection destination drift: view=%#v runtime=%#v err=%v", view, rt, err)
				}
				if err := config.Write(rt.ConfigPath, file); err != nil {
					t.Fatalf("Write(legacy profile): %v", err)
				}
				roundTrip, err := config.ResolveRuntime(config.Options{LookupEnv: fakeEnv(nil), UserConfigDir: userConfigDir(dir), Mode: config.ResolveStrict})
				if err != nil || roundTrip.BaseURL != rt.BaseURL || roundTrip.APIBaseURL != rt.APIBaseURL {
					t.Fatalf("written legacy destination drift: runtime=%#v err=%v", roundTrip, err)
				}
			}
		})
	}
}

func TestEmptyAuthFileDoesNotChangeFreshProductionDefault(t *testing.T) {
	for _, body := range []string{
		`{"version":1,"profiles":{}}`,
		`{"version":1,"profiles":{"other":{"api_key":"ak_other|secret"}}}`,
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "chab", "auth.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		rt, err := config.ResolveRuntime(config.Options{LookupEnv: fakeEnv(nil), UserConfigDir: userConfigDir(dir), Mode: config.ResolveStrict})
		if err != nil || rt.BaseURL != config.DefaultBaseURL || rt.APIBaseURL != config.DefaultAPIBaseURL {
			t.Fatalf("irrelevant auth should keep fresh defaults: runtime=%#v err=%v", rt, err)
		}
	}
}

func TestMalformedAuthHintDoesNotChangeFreshDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chab", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{malformed"), 0o600); err != nil {
		t.Fatal(err)
	}
	rt, err := config.ResolveRuntime(config.Options{
		LookupEnv:     fakeEnv(map[string]string{"CHAB_API_KEY": "ak_env|secret"}),
		UserConfigDir: userConfigDir(dir),
		Mode:          config.ResolveStrict,
	})
	if err != nil || rt.BaseURL != config.DefaultBaseURL || rt.APIBaseURL != config.DefaultAPIBaseURL {
		t.Fatalf("malformed auth hint must not change URL: runtime=%#v err=%v", rt, err)
	}
}

func TestStoredLegacyKeyKeepsDestinationAfterAnotherProfileCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, "chab", "config.yml"), "version: 1\ncurrent_profile: staging\nprofiles:\n  staging:\n    base_url: https://staging.example.test\n")
	if err := os.WriteFile(filepath.Join(dir, "chab", "auth.json"), []byte(`{"version":1,"profiles":{"local":{"api_key":"ak_legacy|secret"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := config.LoadWithAuthFallback(filepath.Join(dir, "chab", "config.yml"), filepath.Join(dir, "chab", "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, persisted, err := config.ResolveProfileView(file, "local")
	if err != nil || persisted || view.BaseURL != config.LegacyImplicitBaseURL || view.APIBaseURL != "http://localhost/v1" {
		t.Fatalf("legacy unsaved profile view=%#v persisted=%t err=%v", view, persisted, err)
	}
	rt, err := config.ResolveRuntime(config.Options{LookupEnv: fakeEnv(map[string]string{"CHAB_PROFILE": "local"}), UserConfigDir: userConfigDir(dir), Mode: config.ResolveForWrite})
	if err != nil || rt.BaseURL != config.LegacyImplicitBaseURL || rt.APIBaseURL != "http://localhost/v1" {
		t.Fatalf("legacy unsaved profile runtime=%#v err=%v", rt, err)
	}
}

func TestConfigFileExistsAccessor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load(missing) error = %v", err)
	}
	if file.Exists() {
		t.Fatalf("missing file Exists() = true")
	}

	if err := config.UpsertProfile(file, "local", config.Profile{BaseURL: "https://example.test"}); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}
	if err := config.SetCurrentProfile(file, "local"); err != nil {
		t.Fatalf("SetCurrentProfile() error = %v", err)
	}
	if err := config.Write(path, file); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if !file.Exists() {
		t.Fatalf("written file Exists() = false")
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load(existing) error = %v", err)
	}
	if !loaded.Exists() {
		t.Fatalf("loaded file Exists() = false")
	}
}

func TestResolveRuntimePathPrecedence(t *testing.T) {
	dir := t.TempDir()
	envConfig := filepath.Join(dir, "env-config.yml")
	envAuth := filepath.Join(dir, "env-auth.json")
	flagConfig := filepath.Join(dir, "flag-config.yml")
	flagAuth := filepath.Join(dir, "flag-auth.json")

	rt, err := config.ResolveRuntime(config.Options{
		LookupEnv: fakeEnv(map[string]string{
			"CHAB_CONFIG":    envConfig,
			"CHAB_AUTH_FILE": envAuth,
		}),
		UserConfigDir: userConfigDir(dir),
	})
	if err != nil {
		t.Fatalf("ResolveRuntime() env paths error = %v", err)
	}
	if rt.ConfigPath != envConfig || rt.AuthPath != envAuth {
		t.Fatalf("env paths not used: %#v", rt)
	}

	rt, err = config.ResolveRuntime(config.Options{
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
		t.Fatalf("ResolveRuntime() flag paths error = %v", err)
	}
	if rt.ConfigPath != flagConfig || rt.AuthPath != flagAuth {
		t.Fatalf("flag paths did not beat env paths: %#v", rt)
	}
}

func TestResolveRuntimeEmptyEnvironmentValuesAreUnset(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, "chab", "config.yml"), `
version: 1
current_profile: file
profiles:
  file:
    base_url: https://file.example.test/app
    api_base_url: https://api-file.example.test/v1
    locale: de
    default_output: table
    defaults:
      project_list_limit: 33
`)

	rt, err := config.ResolveRuntime(config.Options{
		LookupEnv: fakeEnv(map[string]string{
			"CHAB_PROFILE":      "",
			"CHAB_BASE_URL":     "",
			"CHAB_API_BASE_URL": "",
			"CHAB_LOCALE":       "",
			"CHAB_CONFIG":       "",
			"CHAB_AUTH_FILE":    "",
		}),
		UserConfigDir: userConfigDir(dir),
	})
	if err != nil {
		t.Fatalf("ResolveRuntime() error = %v", err)
	}
	if rt.Profile != "file" || rt.BaseURL != "https://file.example.test/app" || rt.APIBaseURL != "https://api-file.example.test/v1" {
		t.Fatalf("empty env values did not fall through to file/default values: %#v", rt)
	}
	if rt.Locale != "de" || rt.ProjectListLimit != 33 {
		t.Fatalf("empty env values disturbed carried defaults: %#v", rt)
	}
	if rt.ConfigPath != filepath.Join(dir, "chab", "config.yml") || rt.AuthPath != filepath.Join(dir, "chab", "auth.json") {
		t.Fatalf("empty path env values did not fall through to default paths: %#v", rt)
	}
}

func TestResolveRuntimeProfileAndSettingPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, "chab", "config.yml"), `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://file.example.test/app
    api_base_url: https://api-file.example.test/v1
    locale: en
    default_output: table
    defaults:
      project_list_limit: 41
  staging:
    base_url: https://staging-file.example.test
    locale: de
    default_output: table
    defaults:
      project_list_limit: 42
`)

	rt, err := config.ResolveRuntime(config.Options{
		Flags: config.FlagOverrides{
			Profile: config.OverrideString{Value: "local", Set: true},
			BaseURL: config.OverrideString{Value: "https://flag.example.test/root", Set: true},
			Locale:  config.OverrideString{Value: "en", Set: true},
		},
		LookupEnv: fakeEnv(map[string]string{
			"CHAB_PROFILE":  "staging",
			"CHAB_BASE_URL": "https://env.example.test",
			"CHAB_LOCALE":   "de",
		}),
		UserConfigDir: userConfigDir(dir),
	})
	if err != nil {
		t.Fatalf("ResolveRuntime() error = %v", err)
	}
	if rt.Profile != "local" {
		t.Fatalf("Profile = %q, want local", rt.Profile)
	}
	if rt.BaseURL != "https://flag.example.test/root" {
		t.Fatalf("BaseURL = %q", rt.BaseURL)
	}
	if rt.APIBaseURL != "https://flag.example.test/root/v1" {
		t.Fatalf("APIBaseURL = %q", rt.APIBaseURL)
	}
	if rt.Locale != "en" || rt.ProjectListLimit != 41 {
		t.Fatalf("runtime did not preserve selected profile/env/flag precedence: %#v", rt)
	}
}

func TestResolveRuntimeProfileSource(t *testing.T) {
	t.Run("flag", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, filepath.Join(dir, "chab", "config.yml"), `
version: 1
current_profile: current
profiles:
  current:
    base_url: https://current.example.test
`)
		rt, err := config.ResolveRuntime(config.Options{
			Flags:         config.FlagOverrides{Profile: config.OverrideString{Value: "flagged", Set: true}},
			LookupEnv:     fakeEnv(map[string]string{"CHAB_PROFILE": "env"}),
			UserConfigDir: userConfigDir(dir),
			Mode:          config.ResolveForWrite,
		})
		if err != nil {
			t.Fatalf("ResolveRuntime(flag) error = %v", err)
		}
		if rt.Profile != "flagged" || rt.ProfileSource != config.ProfileSourceFlag {
			t.Fatalf("runtime = %#v, want flag source", rt)
		}
	})

	t.Run("env", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, filepath.Join(dir, "chab", "config.yml"), `
version: 1
current_profile: current
profiles:
  current:
    base_url: https://current.example.test
`)
		rt, err := config.ResolveRuntime(config.Options{
			LookupEnv:     fakeEnv(map[string]string{"CHAB_PROFILE": "env"}),
			UserConfigDir: userConfigDir(dir),
			Mode:          config.ResolveForWrite,
		})
		if err != nil {
			t.Fatalf("ResolveRuntime(env) error = %v", err)
		}
		if rt.Profile != "env" || rt.ProfileSource != config.ProfileSourceEnv {
			t.Fatalf("runtime = %#v, want env source", rt)
		}
	})

	t.Run("current", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, filepath.Join(dir, "chab", "config.yml"), `
version: 1
current_profile: current
profiles:
  current:
    base_url: https://current.example.test
`)
		rt, err := config.ResolveRuntime(config.Options{
			LookupEnv:     fakeEnv(nil),
			UserConfigDir: userConfigDir(dir),
			Mode:          config.ResolveForWrite,
		})
		if err != nil {
			t.Fatalf("ResolveRuntime(current) error = %v", err)
		}
		if rt.Profile != "current" || rt.ProfileSource != config.ProfileSourceCurrent {
			t.Fatalf("runtime = %#v, want current source", rt)
		}
	})

	t.Run("default", func(t *testing.T) {
		dir := t.TempDir()
		rt, err := config.ResolveRuntime(config.Options{
			LookupEnv:     fakeEnv(nil),
			UserConfigDir: userConfigDir(dir),
			Mode:          config.ResolveForWrite,
		})
		if err != nil {
			t.Fatalf("ResolveRuntime(default) error = %v", err)
		}
		if rt.Profile != "local" || rt.ProfileSource != config.ProfileSourceDefault {
			t.Fatalf("runtime = %#v, want default source", rt)
		}
	})
}

func TestResolveRuntimeStrictMissingProfileModes(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, "chab", "config.yml"), `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://file.example.test
    default_output: table
    defaults:
      project_list_limit: 30
`)

	_, err := config.ResolveRuntime(config.Options{
		Flags:         config.FlagOverrides{Profile: config.OverrideString{Value: "missing", Set: true}},
		LookupEnv:     fakeEnv(nil),
		UserConfigDir: userConfigDir(dir),
		Mode:          config.ResolveStrict,
	})
	requireConfigKind(t, err, config.ErrMissingProfile, 1)

	rt, err := config.ResolveRuntime(config.Options{
		Flags:         config.FlagOverrides{Profile: config.OverrideString{Value: "missing", Set: true}},
		LookupEnv:     fakeEnv(nil),
		UserConfigDir: userConfigDir(dir),
		Mode:          config.ResolveForWrite,
	})
	if err != nil {
		t.Fatalf("ResolveRuntime(write) error = %v", err)
	}
	if rt.Profile != "missing" || rt.BaseURL != config.DefaultBaseURL || rt.APIBaseURL != config.DefaultAPIBaseURL {
		t.Fatalf("write-mode runtime = %#v", rt)
	}

	// Strict mode normally requires the selected profile, but a URL override
	// gives callers enough runtime data to proceed without the missing entry.
	rt, err = config.ResolveRuntime(config.Options{
		Flags: config.FlagOverrides{
			Profile: config.OverrideString{Value: "missing", Set: true},
			BaseURL: config.OverrideString{Value: "https://override.example.test/base", Set: true},
		},
		LookupEnv:     fakeEnv(nil),
		UserConfigDir: userConfigDir(dir),
		Mode:          config.ResolveStrict,
	})
	if err != nil {
		t.Fatalf("ResolveRuntime(strict with URL override) error = %v", err)
	}
	if rt.APIBaseURL != "https://override.example.test/base/v1" {
		t.Fatalf("APIBaseURL = %q", rt.APIBaseURL)
	}

	rt, err = config.ResolveRuntime(config.Options{
		Flags: config.FlagOverrides{
			Profile: config.OverrideString{Value: "missing", Set: true},
		},
		LookupEnv: fakeEnv(map[string]string{
			"CHAB_BASE_URL": "https://env-override.example.test/base",
		}),
		UserConfigDir: userConfigDir(dir),
		Mode:          config.ResolveStrict,
	})
	if err != nil {
		t.Fatalf("ResolveRuntime(strict with env URL override) error = %v", err)
	}
	if rt.APIBaseURL != "https://env-override.example.test/base/v1" {
		t.Fatalf("env override APIBaseURL = %q", rt.APIBaseURL)
	}

	rt, err = config.ResolveRuntime(config.Options{
		Flags: config.FlagOverrides{
			Profile:    config.OverrideString{Value: "missing", Set: true},
			APIBaseURL: config.OverrideString{Value: "https://api-override.example.test/v1", Set: true},
		},
		LookupEnv:     fakeEnv(nil),
		UserConfigDir: userConfigDir(dir),
		Mode:          config.ResolveStrict,
	})
	if err != nil {
		t.Fatalf("ResolveRuntime(strict with API URL flag override) error = %v", err)
	}
	if rt.BaseURL != config.DefaultBaseURL || rt.APIBaseURL != "https://api-override.example.test/v1" {
		t.Fatalf("API flag override runtime = %#v", rt)
	}

	rt, err = config.ResolveRuntime(config.Options{
		Flags: config.FlagOverrides{
			Profile: config.OverrideString{Value: "missing", Set: true},
		},
		LookupEnv: fakeEnv(map[string]string{
			"CHAB_API_BASE_URL": "https://api-env-override.example.test/v1",
		}),
		UserConfigDir: userConfigDir(dir),
		Mode:          config.ResolveStrict,
	})
	if err != nil {
		t.Fatalf("ResolveRuntime(strict with API URL env override) error = %v", err)
	}
	if rt.BaseURL != config.DefaultBaseURL || rt.APIBaseURL != "https://api-env-override.example.test/v1" {
		t.Fatalf("API env override runtime = %#v", rt)
	}
}

func TestResolveRuntimeAPIBaseURLCrossFieldRules(t *testing.T) {
	dir := t.TempDir()
	var resolved []config.Runtime
	writeConfig(t, filepath.Join(dir, "chab", "config.yml"), `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://example.test/app/
    api_base_url: https://api.example.test/v1/
    locale: en
    default_output: table
    defaults:
      project_list_limit: 30
`)

	rt, err := config.ResolveRuntime(config.Options{LookupEnv: fakeEnv(nil), UserConfigDir: userConfigDir(dir)})
	if err != nil {
		t.Fatalf("ResolveRuntime(profile api) error = %v", err)
	}
	resolved = append(resolved, rt)
	if rt.BaseURL != "https://example.test/app" || rt.APIBaseURL != "https://api.example.test/v1" ||
		rt.BaseURLSource != config.RuntimeValueSourceFile || rt.APIBaseURLSource != config.RuntimeValueSourceFile {
		t.Fatalf("profile API base did not survive/normalize: %#v", rt)
	}

	rt, err = config.ResolveRuntime(config.Options{
		Flags:         config.FlagOverrides{BaseURL: config.OverrideString{Value: "https://staging.example.test", Set: true}},
		LookupEnv:     fakeEnv(nil),
		UserConfigDir: userConfigDir(dir),
	})
	if err != nil {
		t.Fatalf("ResolveRuntime(flag base) error = %v", err)
	}
	resolved = append(resolved, rt)
	if rt.APIBaseURL != "https://staging.example.test/v1" ||
		rt.BaseURLSource != config.RuntimeValueSourceFlag || rt.APIBaseURLSource != config.RuntimeValueSourceDerived {
		t.Fatalf("flag base did not re-derive API base: %#v", rt)
	}

	rt, err = config.ResolveRuntime(config.Options{
		LookupEnv: fakeEnv(map[string]string{
			"CHAB_BASE_URL": "https://env-base.example.test/app",
		}),
		UserConfigDir: userConfigDir(dir),
	})
	if err != nil {
		t.Fatalf("ResolveRuntime(env base) error = %v", err)
	}
	resolved = append(resolved, rt)
	if rt.APIBaseURL != "https://env-base.example.test/app/v1" ||
		rt.BaseURLSource != config.RuntimeValueSourceEnv || rt.APIBaseURLSource != config.RuntimeValueSourceDerived {
		t.Fatalf("env base did not re-derive API base: %#v", rt)
	}

	rt, err = config.ResolveRuntime(config.Options{
		Flags: config.FlagOverrides{
			BaseURL: config.OverrideString{Value: "https://staging.example.test", Set: true},
		},
		LookupEnv: fakeEnv(map[string]string{
			"CHAB_API_BASE_URL": "https://explicit.example.test/api",
		}),
		UserConfigDir: userConfigDir(dir),
	})
	if err != nil {
		t.Fatalf("ResolveRuntime(env api) error = %v", err)
	}
	resolved = append(resolved, rt)
	if rt.APIBaseURL != "https://explicit.example.test/api" || rt.APIBaseURLSource != config.RuntimeValueSourceEnv {
		t.Fatalf("explicit API env did not win: %#v", rt)
	}

	rt, err = config.ResolveRuntime(config.Options{
		Flags: config.FlagOverrides{
			BaseURL:    config.OverrideString{Value: "https://flag-base.example.test", Set: true},
			APIBaseURL: config.OverrideString{Value: "https://flag-api.example.test/root", Set: true},
		},
		LookupEnv: fakeEnv(map[string]string{
			"CHAB_API_BASE_URL": "https://env-api.example.test/root",
		}),
		UserConfigDir: userConfigDir(dir),
	})
	if err != nil {
		t.Fatalf("ResolveRuntime(flag api) error = %v", err)
	}
	resolved = append(resolved, rt)
	if rt.APIBaseURL != "https://flag-api.example.test/root" || rt.APIBaseURLSource != config.RuntimeValueSourceFlag {
		t.Fatalf("explicit API flag did not win: %#v", rt)
	}

	pathDir := t.TempDir()
	writeConfig(t, filepath.Join(pathDir, "chab", "config.yml"), `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://example.test/app/
    default_output: table
    defaults:
      project_list_limit: 30
`)
	rt, err = config.ResolveRuntime(config.Options{LookupEnv: fakeEnv(nil), UserConfigDir: userConfigDir(pathDir)})
	if err != nil {
		t.Fatalf("ResolveRuntime(derived base path) error = %v", err)
	}
	resolved = append(resolved, rt)
	if rt.APIBaseURL != "https://example.test/app/v1" ||
		rt.BaseURLSource != config.RuntimeValueSourceFile || rt.APIBaseURLSource != config.RuntimeValueSourceDerived {
		t.Fatalf("derived API base did not preserve base path: %#v", rt)
	}

	defaultDir := t.TempDir()
	rt, err = config.ResolveRuntime(config.Options{LookupEnv: fakeEnv(nil), UserConfigDir: userConfigDir(defaultDir)})
	if err != nil {
		t.Fatalf("ResolveRuntime(default derivation) error = %v", err)
	}
	resolved = append(resolved, rt)
	if rt.APIBaseURL != config.DefaultAPIBaseURL ||
		rt.BaseURLSource != config.RuntimeValueSourceDefault ||
		rt.APIBaseURLSource != config.RuntimeValueSourceDerived {
		t.Fatalf("default API derivation = %#v", rt)
	}

	equalDir := t.TempDir()
	writeConfig(t, filepath.Join(equalDir, "chab", "config.yml"), `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://equal.example.test
    api_base_url: https://equal.example.test/v1
`)
	rt, err = config.ResolveRuntime(config.Options{LookupEnv: fakeEnv(nil), UserConfigDir: userConfigDir(equalDir)})
	if err != nil {
		t.Fatalf("ResolveRuntime(explicit equal API) error = %v", err)
	}
	resolved = append(resolved, rt)
	if rt.APIBaseURLSource != config.RuntimeValueSourceFile {
		t.Fatalf("explicit equal API source = %v, want file", rt.APIBaseURLSource)
	}
	for _, resolvedRuntime := range resolved {
		if resolvedRuntime.BaseURLSource == config.RuntimeValueSourceDerived {
			t.Fatalf("base URL source must never be derived: %#v", resolvedRuntime)
		}
	}
}

func TestResolveRuntimeValidationErrors(t *testing.T) {
	tests := []struct {
		name  string
		flags config.FlagOverrides
		env   map[string]string
		body  string
		kind  config.ErrorKind
	}{
		{name: "invalid profile", flags: config.FlagOverrides{Profile: config.OverrideString{Value: "Bad", Set: true}}, kind: config.ErrInvalidProfileName},
		{name: "blank profile flag", flags: config.FlagOverrides{Profile: config.OverrideString{Value: "", Set: true}}, kind: config.ErrInvalidProfileName},
		{name: "blank config path flag", flags: config.FlagOverrides{ConfigPath: config.OverrideString{Value: "", Set: true}}, kind: config.ErrMalformedConfig},
		{name: "blank auth path flag", flags: config.FlagOverrides{AuthPath: config.OverrideString{Value: "", Set: true}}, kind: config.ErrMalformedConfig},
		{name: "invalid base url", flags: config.FlagOverrides{BaseURL: config.OverrideString{Value: "ftp://example.test", Set: true}}, kind: config.ErrInvalidURL},
		{name: "empty query base url", flags: config.FlagOverrides{BaseURL: config.OverrideString{Value: "https://example.test?", Set: true}}, kind: config.ErrInvalidURL},
		{name: "invalid api url", flags: config.FlagOverrides{APIBaseURL: config.OverrideString{Value: "https://user@example.test", Set: true}}, kind: config.ErrInvalidURL},
		{name: "invalid locale", flags: config.FlagOverrides{Locale: config.OverrideString{Value: "fr", Set: true}}, kind: config.ErrInvalidLocale},
		{name: "blank locale flag", flags: config.FlagOverrides{Locale: config.OverrideString{Value: "", Set: true}}, kind: config.ErrInvalidLocale},
		{name: "invalid file locale", body: validConfigProfile("locale: fr"), kind: config.ErrInvalidLocale},
		{name: "invalid output", body: validConfigProfile("default_output: json"), kind: config.ErrInvalidDefault},
		{name: "invalid list limit", body: validConfigProfile("defaults:\n      project_list_limit: 0"), kind: config.ErrInvalidDefault},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if test.body != "" {
				writeConfig(t, filepath.Join(dir, "chab", "config.yml"), test.body)
			}
			_, err := config.ResolveRuntime(config.Options{
				Flags:         test.flags,
				LookupEnv:     fakeEnv(test.env),
				UserConfigDir: userConfigDir(dir),
			})
			requireConfigKind(t, err, test.kind, 1)
		})
	}
}

func TestResolveRuntimeRejectsInvalidURLShapes(t *testing.T) {
	// Include bare marker cases because url.Parse otherwise normalizes empty
	// query and fragment markers into shapes that look harmless after parsing.
	tests := []struct {
		name  string
		value string
	}{
		{name: "userinfo", value: "https://user:pass@example.test"},
		{name: "query", value: "https://example.test/path?x=1"},
		{name: "bare query marker", value: "https://example.test?"},
		{name: "fragment", value: "https://example.test/path#section"},
		{name: "bare fragment marker", value: "https://example.test#"},
		{name: "missing host", value: "https:///path"},
		{name: "invalid scheme", value: "ftp://example.test"},
		{name: "empty", value: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := config.ResolveRuntime(config.Options{
				Flags:         config.FlagOverrides{BaseURL: config.OverrideString{Value: test.value, Set: true}},
				LookupEnv:     fakeEnv(nil),
				UserConfigDir: userConfigDir(t.TempDir()),
			})
			requireConfigKind(t, err, config.ErrInvalidURL, 1)
		})
	}
}

func TestLoadMalformedAndUnsupportedConfig(t *testing.T) {
	tests := []struct {
		name string
		body string
		kind config.ErrorKind
	}{
		{name: "malformed yaml", body: "version: [", kind: config.ErrMalformedConfig},
		{name: "empty file", body: " \n", kind: config.ErrMalformedConfig},
		{name: "non mapping", body: "- version\n- 1\n", kind: config.ErrMalformedConfig},
		{name: "missing version", body: "profiles: {}\n", kind: config.ErrMalformedConfig},
		{name: "non numeric version", body: "version: one\nprofiles: {}\n", kind: config.ErrMalformedConfig},
		{name: "unsupported version", body: "version: 2\nprofiles: {}\n", kind: config.ErrUnsupportedVersion},
		{name: "non numeric list limit", body: validConfigProfile("defaults:\n      project_list_limit: nope"), kind: config.ErrMalformedConfig},
		{name: "duplicate top-level key", body: "version: 1\nversion: 2\nprofiles: {}\n", kind: config.ErrMalformedConfig},
		{name: "duplicate profile name", body: "version: 1\nprofiles:\n  local: {}\n  local: {}\n", kind: config.ErrMalformedConfig},
		{name: "duplicate profile field", body: "version: 1\nprofiles:\n  local:\n    base_url: https://one.example.test\n    base_url: https://two.example.test\n", kind: config.ErrMalformedConfig},
		{name: "duplicate nested field", body: "version: 1\nprofiles:\n  local:\n    defaults:\n      project_list_limit: 1\n      project_list_limit: 2\n", kind: config.ErrMalformedConfig},
		{name: "additional document", body: "version: 1\nprofiles: {}\n---\nversion: 1\nprofiles: {}\n", kind: config.ErrMalformedConfig},
		{name: "empty additional document", body: "version: 1\nprofiles: {}\n---\n", kind: config.ErrMalformedConfig},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			writeConfig(t, path, test.body)
			_, err := config.Load(path)
			requireConfigKind(t, err, test.kind, 1)
		})
	}
}

func TestConfigUnknownKeysSurviveWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	writeConfig(t, path, `
version: 1
x_top: keep
current_profile: local
profiles:
  local:
    x_profile: keep
    base_url: https://old.example.test
    api_base_url: https://old.example.test/v1
    locale: en
    default_output: table
    defaults:
      x_default: keep
      project_list_limit: 5
`)

	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := config.UpsertProfile(file, "local", config.Profile{
		BaseURL:       "https://new.example.test/root/",
		Locale:        "de",
		DefaultOutput: "table",
		Defaults:      config.Defaults{ProjectListLimit: 7},
	}); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}
	if err := config.Write(path, file); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data := string(readFile(t, path))
	for _, want := range []string{"x_top: keep", "x_profile: keep", "x_default: keep"} {
		if !strings.Contains(data, want) {
			t.Fatalf("written YAML missing %q:\n%s", want, data)
		}
	}

	rt, err := config.ResolveRuntime(config.Options{
		Flags:         config.FlagOverrides{ConfigPath: config.OverrideString{Value: path, Set: true}},
		LookupEnv:     fakeEnv(nil),
		UserConfigDir: userConfigDir(t.TempDir()),
	})
	if err != nil {
		t.Fatalf("ResolveRuntime() error = %v", err)
	}
	if rt.BaseURL != "https://new.example.test/root" || rt.APIBaseURL != "https://new.example.test/root/v1" || rt.ProjectListLimit != 7 {
		t.Fatalf("runtime after write = %#v", rt)
	}
}

func TestConfigWriteValidationBeforeCreatingFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing", "config.yml")
	err := config.Write(path, &config.File{
		Version:        1,
		CurrentProfile: "Bad",
		Profiles:       map[string]config.Profile{},
	})
	requireConfigKind(t, err, config.ErrInvalidProfileName, 1)
	if _, statErr := os.Stat(filepath.Dir(path)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Write created directory before validation failed: %v", statErr)
	}
}

func TestConfigWriteFailureCleansTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	file := &config.File{}
	if err := config.UpsertProfile(file, "local", config.Profile{BaseURL: "https://file.example.test"}); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}
	err := config.Write(path, file)
	requireConfigKind(t, err, config.ErrMalformedConfig, 1)
	matches, globErr := filepath.Glob(filepath.Join(dir, ".config.yml.tmp-*"))
	if globErr != nil {
		t.Fatalf("Glob() error = %v", globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("Write left temp files after failed replacement: %#v", matches)
	}
}

func TestConfigErrorsRedactSecretShapedStrings(t *testing.T) {
	_, err := config.ResolveRuntime(config.Options{
		Flags: config.FlagOverrides{
			Profile: config.OverrideString{Value: "ak_bad|super-secret", Set: true},
		},
		LookupEnv:     fakeEnv(nil),
		UserConfigDir: userConfigDir(t.TempDir()),
	})
	requireConfigKind(t, err, config.ErrInvalidProfileName, 1)
	if strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("config error leaked secret-shaped value: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("config error did not show redaction token: %v", err)
	}

	path := filepath.Join(t.TempDir(), "config.yml")
	writeConfig(t, path, `
version: 1
profiles:
  "ak_bad|file-secret": null
`)
	_, err = config.Load(path)
	requireConfigKind(t, err, config.ErrMalformedConfig, 1)
	if strings.Contains(err.Error(), "file-secret") {
		t.Fatalf("config load error leaked secret-shaped field value: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("config load error did not show redaction token: %v", err)
	}
}

func TestConfigErrorFieldsRedactContextAddedDuringResolution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ak_path%7Cpath-secret", "config.yml")
	_, err := config.ResolveRuntime(config.Options{
		Flags: config.FlagOverrides{
			ConfigPath: config.OverrideString{Value: path, Set: true},
			BaseURL:    config.OverrideString{Value: "ftp://example.test", Set: true},
		},
		LookupEnv:     fakeEnv(nil),
		UserConfigDir: userConfigDir(t.TempDir()),
	})
	requireConfigKind(t, err, config.ErrInvalidURL, 1)

	var configErr *config.Error
	if !errors.As(err, &configErr) {
		t.Fatalf("error type = %T, want *config.Error", err)
	}
	if strings.Contains(configErr.Path, "path-secret") || strings.Contains(err.Error(), "path-secret") {
		t.Fatalf("config error context leaked secret-shaped path: field=%q error=%v", configErr.Path, err)
	}
	if !strings.Contains(configErr.Path, "[REDACTED]") {
		t.Fatalf("config error path field was not redacted: %#v", configErr)
	}
}

func TestConfigWritePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode assertions are not portable to Windows")
	}

	path := filepath.Join(t.TempDir(), "chab", "config.yml")
	file := &config.File{}
	if err := config.UpsertProfile(file, "local", config.Profile{
		BaseURL:       "https://file.example.test",
		DefaultOutput: "table",
		Defaults:      config.Defaults{ProjectListLimit: 30},
	}); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}
	if err := config.SetCurrentProfile(file, "local"); err != nil {
		t.Fatalf("SetCurrentProfile() error = %v", err)
	}
	if err := config.Write(path, file); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	assertMode(t, filepath.Dir(path), 0o700)
	assertMode(t, path, 0o644)
}

func validConfigProfile(replacement string) string {
	if replacement == "" {
		replacement = "locale: en\n    default_output: table\n    defaults:\n      project_list_limit: 30"
	}
	return `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://file.example.test
    api_base_url: https://file.example.test/v1
    ` + replacement + `
`
}

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimPrefix(body, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return data
}

func fakeEnv(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func userConfigDir(dir string) func() (string, error) {
	return func() (string, error) {
		return dir, nil
	}
}

func requireConfigKind(t *testing.T, err error, kind config.ErrorKind, exitCode int) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want kind %s", kind)
	}
	var configErr *config.Error
	if !errors.As(err, &configErr) {
		t.Fatalf("error type = %T, want *config.Error: %v", err, err)
	}
	if configErr.Kind != kind {
		t.Fatalf("error kind = %s, want %s: %v", configErr.Kind, kind, err)
	}
	coder, ok := err.(interface{ ExitCode() int })
	if !ok {
		t.Fatalf("error does not implement ExitCode(): %T", err)
	}
	if got := coder.ExitCode(); got != exitCode {
		t.Fatalf("ExitCode() = %d, want %d", got, exitCode)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode %s = %04o, want %04o", path, got, want)
	}
}

func modeName(mode config.ResolveMode) string {
	if mode == config.ResolveForWrite {
		return "write"
	}
	return "strict"
}
