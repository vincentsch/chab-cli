package rungradadapter_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/rungradadapter"
	rgconfig "github.com/vincentsch/rungrad/config"
)

func TestConfig(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		got, err := rungradadapter.Config(filepath.Join(t.TempDir(), "missing.yml"))
		if err != nil {
			t.Fatalf("Config() error = %v", err)
		}
		if got.Version != 1 || got.CurrentProfile != "" || got.Profiles != nil {
			t.Fatalf("Config(missing) = %#v, want empty v1 config", got)
		}
	})

	t.Run("base url only derives api service", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yml")
		writeConfig(t, path, `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://example.test/app
`)
		got, err := rungradadapter.Config(path)
		if err != nil {
			t.Fatalf("Config() error = %v", err)
		}
		profile := got.Profiles["local"]
		if profile.BaseURL != "https://example.test/app" {
			t.Fatalf("BaseURL = %q", profile.BaseURL)
		}
		if profile.Services["api_base_url"] != "https://example.test/app/v1" {
			t.Fatalf("api_base_url service = %q", profile.Services["api_base_url"])
		}
	})

	t.Run("explicit api service passes through", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yml")
		writeConfig(t, path, `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://example.test/app
    api_base_url: https://api.example.test/v1
`)
		got, err := rungradadapter.Config(path)
		if err != nil {
			t.Fatalf("Config() error = %v", err)
		}
		if got.Profiles["local"].Services["api_base_url"] != "https://api.example.test/v1" {
			t.Fatalf("api_base_url service = %q", got.Profiles["local"].Services["api_base_url"])
		}
	})

	t.Run("malformed file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yml")
		writeConfig(t, path, "version: [\n")
		_, err := rungradadapter.Config(path)
		requireConfigKind(t, err, config.ErrMalformedConfig)
	})
}

func TestResolvedProjectsRuntime(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		dir := t.TempDir()
		rt, err := config.ResolveRuntime(config.Options{
			LookupEnv:     fakeEnv(nil),
			UserConfigDir: userConfigDir(dir),
		})
		if err != nil {
			t.Fatalf("ResolveRuntime() error = %v", err)
		}
		got := rungradadapter.Resolved(rt)
		if got.Profile != "local" {
			t.Fatalf("Profile = %q, want local", got.Profile)
		}
		if got.ConfigPath != filepath.Join(dir, "chab", "config.yml") {
			t.Fatalf("ConfigPath = %q", got.ConfigPath)
		}
		if got.AuthFilePath != filepath.Join(dir, "chab", "auth.json") {
			t.Fatalf("AuthFilePath = %q", got.AuthFilePath)
		}
		if serviceValue(got, "base_url") != config.DefaultBaseURL || serviceValue(got, "api_base_url") != config.DefaultAPIBaseURL {
			t.Fatalf("default services = %#v", got.Services)
		}
	})

	t.Run("precedence and api derivation", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, filepath.Join(dir, "chab", "config.yml"), `
version: 1
current_profile: local
profiles:
  local:
    base_url: https://profile.example.test/base
    api_base_url: https://profile-api.example.test/v1
    locale: en
`)
		rt, err := config.ResolveRuntime(config.Options{
			Flags: config.FlagOverrides{
				Profile: config.OverrideString{Value: "local", Set: true},
				BaseURL: config.OverrideString{Value: "https://flag.example.test/root", Set: true},
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
		got := rungradadapter.Resolved(rt)
		if got.Profile != "local" {
			t.Fatalf("Profile = %q", got.Profile)
		}
		if serviceValue(got, "base_url") != "https://flag.example.test/root" {
			t.Fatalf("base_url service = %q", serviceValue(got, "base_url"))
		}
		if serviceValue(got, "api_base_url") != "https://flag.example.test/root/v1" {
			t.Fatalf("api_base_url service = %q", serviceValue(got, "api_base_url"))
		}
		if serviceValue(got, "locale") != "de" {
			t.Fatalf("locale service = %q", serviceValue(got, "locale"))
		}
	})
}

func TestValidateServiceURL(t *testing.T) {
	if err := rungradadapter.ValidateServiceURL("https://example.test/root"); err != nil {
		t.Fatalf("ValidateServiceURL(valid) error = %v", err)
	}
	err := rungradadapter.ValidateServiceURL("ftp://example.test")
	var configErr *config.Error
	if !errors.As(err, &configErr) || configErr.Kind != config.ErrInvalidURL {
		t.Fatalf("ValidateServiceURL(invalid) error = %T %v, want invalid URL config error", err, err)
	}
}

func serviceValue(resolved rgconfig.Resolved, name string) string {
	service, ok := resolved.Services[name]
	if !ok {
		return ""
	}
	return service.Value
}

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func fakeEnv(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func userConfigDir(dir string) func() (string, error) {
	return func() (string, error) { return dir, nil }
}

func requireConfigKind(t *testing.T, err error, kind config.ErrorKind) {
	t.Helper()
	var configErr *config.Error
	if !errors.As(err, &configErr) || configErr.Kind != kind {
		t.Fatalf("error = %T %v, want config kind %s", err, err, kind)
	}
}
