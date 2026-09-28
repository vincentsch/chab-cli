package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

import (
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/config"
)

// State holds per-test local CLI state paths under a temp directory.
type State struct {
	Dir        string
	ConfigPath string
	AuthPath   string
}

// NewState allocates isolated local config and auth paths for one test.
func NewState(t *testing.T) State {
	t.Helper()
	dir := t.TempDir()
	return State{
		Dir:        dir,
		ConfigPath: filepath.Join(dir, "config.yml"),
		AuthPath:   filepath.Join(dir, "auth.json"),
	}
}

// Args prepends --config and --auth-file so a command cannot touch the
// developer's real chab state.
func (s State) Args(args ...string) []string {
	out := []string{"--config", s.ConfigPath, "--auth-file", s.AuthPath}
	out = append(out, args...)
	return out
}

// APIArgs additionally prepends --api-base-url <server>/v1 and pins the
// profile/locale used by resource-command fixture calls so host CHAB_PROFILE
// or CHAB_LOCALE values cannot hijack isolated API tests.
func (s State) APIArgs(server *APIServer, args ...string) []string {
	out := []string{
		"--config", s.ConfigPath,
		"--auth-file", s.AuthPath,
		"--profile", "local",
		"--locale", "en",
		"--api-base-url", server.APIBaseURL(),
	}
	out = append(out, args...)
	return out
}

// WriteConfigProfile upserts one profile and optionally selects it.
func WriteConfigProfile(t *testing.T, path, name string, profile config.Profile, setCurrent bool) {
	t.Helper()
	// Tests that omit a product origin must never inherit a production URL.
	// Explicit origins remain untouched for destination/precedence coverage.
	if profile.BaseURL == "" {
		profile.BaseURL = "http://127.0.0.1:9"
	}
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load(%q) error = %v", path, err)
	}
	if err := config.UpsertProfile(file, name, profile); err != nil {
		t.Fatalf("config.UpsertProfile(%q) error = %v", name, err)
	}
	if setCurrent {
		if err := config.SetCurrentProfile(file, name); err != nil {
			t.Fatalf("config.SetCurrentProfile(%q) error = %v", name, err)
		}
	}
	if err := config.Write(path, file); err != nil {
		t.Fatalf("config.Write(%q) error = %v", path, err)
	}
}

// WriteAuthProfile stores one profile credential.
func WriteAuthProfile(t *testing.T, path, profile string, record auth.ProfileAuth) {
	t.Helper()
	file, _, err := auth.Load(path)
	if err != nil {
		t.Fatalf("auth.Load(%q) error = %v", path, err)
	}
	if err := auth.PutProfile(file, profile, record); err != nil {
		t.Fatalf("auth.PutProfile(%q) error = %v", profile, err)
	}
	if err := auth.Write(path, file); err != nil {
		t.Fatalf("auth.Write(%q) error = %v", path, err)
	}
}

// AuthRecord returns the minimal stored-credential fixture used across command
// tests.
func AuthRecord(key string) auth.ProfileAuth {
	return auth.ProfileAuth{
		APIKey:    key,
		DisplayID: "ak_display",
		KeyName:   "Local CLI",
	}
}

// WriteMalformedConfig writes invalid YAML that config.Load rejects.
func WriteMalformedConfig(t *testing.T, path string) {
	t.Helper()
	writeFixtureFile(t, path, []byte("version: [\n"))
}

// WriteMalformedAuth writes invalid JSON that auth.Load rejects.
func WriteMalformedAuth(t *testing.T, path string) {
	t.Helper()
	writeFixtureFile(t, path, []byte(`{"version":`+"\n"))
}

// WriteRawConfig writes an exact secret-free YAML fixture so command tests can
// exercise comments, ordering, scalar style, and omitted known fields.
func WriteRawConfig(t *testing.T, path string, data []byte) {
	t.Helper()
	writeFixtureFile(t, path, append([]byte(nil), data...))
}

func writeFixtureFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

// FakeKey builds the standard fake credential "ak_<id>|ssssssssssssssss".
func FakeKey(id string) string {
	if len(id) < 3 {
		panic("testutil.FakeKey id must be at least three characters")
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			continue
		}
		panic("testutil.FakeKey id must match [A-Za-z0-9_-]+")
	}
	return "ak_" + id + "|" + strings.Repeat("s", 16)
}
