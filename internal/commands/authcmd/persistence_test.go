package authcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/redact"
)

func TestPersistValidatedCredentialSkipsNoOpWriterAndRollback(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yml")
	authPath := filepath.Join(dir, "auth.json")
	original := []byte("version: 1\ncurrent_profile: local\nprofiles:\n    local: {}\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	if err := os.WriteFile(authPath, []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile(auth) error = %v", err)
	}
	writerCalls := 0
	err := persistValidatedCredential(
		testCommand(),
		testFactory(),
		config.Runtime{ConfigPath: configPath, AuthPath: authPath},
		"local",
		api.WhoamiData{},
		"credential",
		false,
		"setup",
		func(*config.File) (bool, error) { return false, nil },
		func(string, *config.File) error {
			writerCalls++
			return nil
		},
	)
	if err == nil {
		t.Fatal("persistValidatedCredential() error = nil")
	}
	if writerCalls != 0 {
		t.Fatalf("writer calls = %d, want 0", writerCalls)
	}
	assertPersistenceBytes(t, configPath, original)
}

func TestPersistValidatedCredentialRollsBackAfterAuthSideFailures(t *testing.T) {
	for _, test := range []struct {
		name        string
		profile     string
		prepareAuth func(t *testing.T, path string)
	}{
		{
			name:    "auth load",
			profile: "local",
			prepareAuth: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
					t.Fatalf("WriteFile(auth) error = %v", err)
				}
			},
		},
		{
			name:        "auth record validation",
			profile:     "INVALID PROFILE",
			prepareAuth: func(*testing.T, string) {},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config.yml")
			authPath := filepath.Join(dir, "auth.json")
			original := []byte("version: 1\ncurrent_profile: old\nprofiles:\n    old: {}\n")
			if err := os.WriteFile(configPath, original, 0o600); err != nil {
				t.Fatalf("WriteFile(config) error = %v", err)
			}
			test.prepareAuth(t, authPath)
			writerCalls := 0
			err := persistValidatedCredential(
				testCommand(),
				testFactory(),
				config.Runtime{ConfigPath: configPath, AuthPath: authPath},
				test.profile,
				api.WhoamiData{},
				"credential",
				false,
				"setup",
				func(*config.File) (bool, error) { return true, nil },
				func(path string, _ *config.File) error {
					writerCalls++
					return os.WriteFile(path, []byte("mutated\n"), 0o600)
				},
			)
			if err == nil {
				t.Fatal("persistValidatedCredential() error = nil")
			}
			if writerCalls != 1 {
				t.Fatalf("writer calls = %d, want 1", writerCalls)
			}
			assertPersistenceBytes(t, configPath, original)
		})
	}
}

func TestPersistValidatedCredentialRollsBackAfterAuthWriteFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission failure requires Unix permissions")
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yml")
	authDir := filepath.Join(dir, "auth")
	authPath := filepath.Join(authDir, "auth.json")
	original := []byte("version: 1\ncurrent_profile: old\nprofiles:\n    old: {}\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	if err := os.Mkdir(authDir, 0o700); err != nil {
		t.Fatalf("Mkdir(auth) error = %v", err)
	}
	if err := os.WriteFile(authPath, []byte("{\"version\":1,\"profiles\":{}}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(auth) error = %v", err)
	}
	if err := os.Chmod(authDir, 0o500); err != nil {
		t.Fatalf("Chmod(auth dir) error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(authDir, 0o700)
	})

	err := persistValidatedCredential(
		testCommand(),
		testFactory(),
		config.Runtime{ConfigPath: configPath, AuthPath: authPath},
		"local",
		api.WhoamiData{},
		"credential",
		false,
		"setup",
		func(*config.File) (bool, error) { return true, nil },
		func(path string, _ *config.File) error {
			return os.WriteFile(path, []byte("mutated\n"), 0o600)
		},
	)
	if err == nil {
		t.Fatal("persistValidatedCredential() error = nil")
	}
	assertPersistenceBytes(t, configPath, original)
}

func TestCredentialConfigStrategiesKeepDistinctWriteShapes(t *testing.T) {
	for _, test := range []struct {
		name            string
		prepare         configPreparation
		writer          configWriter
		wantDefaults    bool
		wantExplicitAPI bool
	}{
		{
			name: "login normalization",
			prepare: loginConfigPreparation(config.Runtime{
				BaseURL:          "https://example.test",
				APIBaseURL:       "https://example.test/v1",
				Locale:           "en",
				DefaultOutput:    config.DefaultOutput,
				ProjectListLimit: config.DefaultProjectListLimit,
			}, "local"),
			writer:          config.Write,
			wantDefaults:    true,
			wantExplicitAPI: true,
		},
		{
			name: "setup preserving repair",
			prepare: setupConfigPreparation(config.Runtime{
				Profile:          "local",
				BaseURL:          "https://example.test",
				APIBaseURL:       "https://example.test/v1",
				APIBaseURLSource: config.RuntimeValueSourceDerived,
				Locale:           "en",
			}),
			writer:          config.WritePreservingShape,
			wantDefaults:    false,
			wantExplicitAPI: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			file, err := config.Load(path)
			if err != nil {
				t.Fatalf("config.Load() error = %v", err)
			}
			changed, err := test.prepare(file)
			if err != nil || !changed {
				t.Fatalf("prepare() = %t, %v", changed, err)
			}
			if err := test.writer(path, file); err != nil {
				t.Fatalf("writer() error = %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			hasDefaults := bytes.Contains(data, []byte("default_output")) &&
				bytes.Contains(data, []byte("project_list_limit"))
			hasExplicitAPI := bytes.Contains(data, []byte("api_base_url"))
			if hasDefaults != test.wantDefaults || hasExplicitAPI != test.wantExplicitAPI {
				t.Fatalf("config shape defaults=%t api=%t", hasDefaults, hasExplicitAPI)
			}
		})
	}
}

func TestRollbackCredentialConfigKeepsOriginalErrorPriorityAndRedactsWarning(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opaque-config-secret", "config.yml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte("version: 1\ncurrent_profile: old\nprofiles: {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	file, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	snapshot, err := config.CaptureRollbackSnapshot(configPath, file)
	if err != nil {
		t.Fatalf("CaptureRollbackSnapshot() error = %v", err)
	}
	if err := os.Remove(configPath); err != nil {
		t.Fatalf("Remove(config) error = %v", err)
	}
	if err := os.Mkdir(configPath, 0o700); err != nil {
		t.Fatalf("Mkdir(config path) error = %v", err)
	}

	registry := redact.NewRegistry()
	registry.RegisterSecret("opaque-config-secret")
	registry.RegisterSecret("opaque-profile-secret")
	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)
	rollbackCredentialConfig(
		cmd,
		&cmdutil.Factory{Secrets: registry},
		config.Runtime{ConfigPath: configPath},
		"opaque-profile-secret",
		"setup",
		snapshot,
		true,
	)
	got := stderr.String()
	if !strings.Contains(got, "Warning: setup updated config") ||
		!strings.Contains(got, "rollback also failed") {
		t.Fatalf("rollback warning = %q", got)
	}
	for _, secret := range []string{"opaque-config-secret", "opaque-profile-secret"} {
		if strings.Contains(got, secret) {
			t.Fatal("rollback warning exposed registered state")
		}
	}
}

func TestPersistValidatedCredentialReturnsAuthErrorWhenRollbackAlsoFails(t *testing.T) {
	dir := t.TempDir()
	secret := "opaque-rollback-path"
	configPath := filepath.Join(dir, secret, "config.yml")
	authPath := filepath.Join(dir, "auth.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("MkdirAll(config parent) error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte("version: 1\ncurrent_profile: old\nprofiles: {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	if err := os.WriteFile(authPath, []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile(auth) error = %v", err)
	}
	registry := redact.NewRegistry()
	registry.RegisterSecret(secret)
	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)
	err := persistValidatedCredential(
		cmd,
		&cmdutil.Factory{
			Now:     func() time.Time { return time.Unix(1, 0).UTC() },
			Secrets: registry,
		},
		config.Runtime{ConfigPath: configPath, AuthPath: authPath},
		"local",
		api.WhoamiData{},
		"credential",
		false,
		"setup",
		func(*config.File) (bool, error) { return true, nil },
		func(path string, _ *config.File) error {
			if removeErr := os.Remove(path); removeErr != nil {
				return removeErr
			}
			return os.Mkdir(path, 0o700)
		},
	)
	if err == nil || !strings.Contains(err.Error(), "could not parse auth JSON") {
		t.Fatalf("returned error = %v, want original auth load error", err)
	}
	if !strings.Contains(stderr.String(), "rollback also failed") ||
		strings.Contains(stderr.String(), secret) {
		t.Fatalf("rollback warning = %q", stderr.String())
	}
}

func testCommand() *cobra.Command {
	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)
	return cmd
}

func testFactory() *cmdutil.Factory {
	return &cmdutil.Factory{
		Now:     func() time.Time { return time.Unix(1, 0).UTC() },
		Secrets: redact.NewRegistry(),
	}
}

func assertPersistenceBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s was not restored exactly", path)
	}
}
