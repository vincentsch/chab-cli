// Package productisolation proves that the Chab product identity is isolated
// from the inherited VILT reference binary when both run under one OS config
// root.
package productisolation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	referenceCommit = "d23ae24ffc884625b1714b90bf02b68293fa25fd"
	defaultRefSrc   = "/home/vincent/vilt-saas-cli"
)

func TestChabAndVILTUseSeparateProductNamespaces(t *testing.T) {
	repoRoot := findRepoRoot(t)
	tmp := t.TempDir()

	chabBin := filepath.Join(tmp, "chab")
	build(t, repoRoot, chabBin, "./cmd/chab")

	refSrc := os.Getenv("CHAB_VILT_REFERENCE_SRC")
	explicitRef := refSrc != ""
	if refSrc == "" {
		refSrc = defaultRefSrc
	}
	if st, err := os.Stat(refSrc); err != nil || !st.IsDir() {
		t.Skipf("VILT reference source unavailable at %s; set CHAB_VILT_REFERENCE_SRC to run product-isolation evidence", refSrc)
	}
	refCommit := strings.TrimSpace(commandOutput(t, refSrc, "git", "rev-parse", "HEAD"))
	if refCommit != referenceCommit {
		if explicitRef {
			t.Fatalf("VILT reference source at %s is %s, want %s", refSrc, refCommit, referenceCommit)
		}
		t.Skipf("VILT reference source at %s is %s, want %s; set CHAB_VILT_REFERENCE_SRC to an exact checkout", refSrc, refCommit, referenceCommit)
	}

	viltBin := filepath.Join(tmp, "vilt")
	build(t, refSrc, viltBin, "./cmd/vilt")

	configRoot := filepath.Join(tmp, "config-root")
	chabEnv := productEnv(configRoot,
		"VILT_CONFIG="+filepath.Join(tmp, "wrong-vilt-config.yml"),
		"VILT_AUTH_FILE="+filepath.Join(tmp, "wrong-vilt-auth.json"),
		"VILT_PROFILE=wrong-vilt-profile",
		"VILT_BASE_URL=https://vilt.invalid",
		"VILT_API_BASE_URL=https://vilt.invalid/api/v1",
		"VILT_API_KEY=vilt_secret_should_not_affect_chab",
	)
	viltEnv := productEnv(configRoot,
		"CHAB_CONFIG="+filepath.Join(tmp, "wrong-chab-config.yml"),
		"CHAB_AUTH_FILE="+filepath.Join(tmp, "wrong-chab-auth.json"),
		"CHAB_PROFILE=wrong-chab-profile",
		"CHAB_BASE_URL=https://chab.invalid",
		"CHAB_API_BASE_URL=https://chab.invalid/v1",
		"CHAB_API_KEY=chab_secret_should_not_affect_vilt",
	)

	chabPaths := run(t, chabBin, chabEnv, "config", "path", "--plain")
	assertContains(t, chabPaths, filepath.Join(configRoot, "chab", "config.yml"))
	assertContains(t, chabPaths, filepath.Join(configRoot, "chab", "auth.json"))
	assertNotContains(t, chabPaths, "wrong-vilt")

	viltPaths := run(t, viltBin, viltEnv, "config", "path", "--plain")
	assertContains(t, viltPaths, filepath.Join(configRoot, "vilt", "config.yml"))
	assertContains(t, viltPaths, filepath.Join(configRoot, "vilt", "auth.json"))
	assertNotContains(t, viltPaths, "wrong-chab")

	chabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/me" {
			t.Fatalf("chab auth request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, chabWhoamiEnvelope())
	}))
	defer chabServer.Close()
	viltServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/whoami" {
			t.Fatalf("vilt auth request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, viltWhoamiEnvelope())
	}))
	defer viltServer.Close()

	runWithInput(t, viltBin, viltEnv, "vilt_iso_key\n",
		"--api-base-url", viltServer.URL+"/api/v1",
		"login", "--no-prompt", "--yes", "--plain",
	)
	runWithInput(t, chabBin, chabEnv, "chab_iso_key\n",
		"--api-base-url", chabServer.URL+"/v1",
		"login", "--api-key", "--no-prompt", "--yes", "--plain",
	)
	viltSnapshotAfterLogin := snapshotPath(t, filepath.Join(configRoot, "vilt"))
	if len(viltSnapshotAfterLogin) == 0 {
		t.Fatal("VILT login did not populate its product namespace")
	}
	if len(snapshotPath(t, filepath.Join(configRoot, "chab"))) == 0 {
		t.Fatal("Chab login did not populate its product namespace")
	}
	run(t, chabBin, chabEnv, "logout", "--plain")
	if afterLogout := snapshotPath(t, filepath.Join(configRoot, "vilt")); !bytes.Equal(viltSnapshotAfterLogin, afterLogout) {
		t.Fatalf("VILT state changed after Chab logout\nbefore=%q\nafter=%q", viltSnapshotAfterLogin, afterLogout)
	}

	malformed := filepath.Join(tmp, "malformed.yml")
	if err := os.WriteFile(malformed, []byte("version: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	offlineRoot := filepath.Join(tmp, "offline-root")
	offlineEnv := productEnv(offlineRoot,
		"CHAB_CONFIG="+malformed,
		"CHAB_API_KEY=chab_offline_secret",
		"CHAB_BASE_URL=::not-a-url",
	)
	versionJSON := run(t, chabBin, offlineEnv, "version", "--json")
	var version map[string]any
	if err := json.Unmarshal([]byte(versionJSON), &version); err != nil {
		t.Fatalf("version output is not JSON: %v\n%s", err, versionJSON)
	}
	assertNotContains(t, versionJSON, "chab_offline_secret")
	if _, err := os.Stat(filepath.Join(offlineRoot, "chab")); !os.IsNotExist(err) {
		t.Fatalf("offline version created chab state: %v", err)
	}

	beforeVILT := snapshotPath(t, filepath.Join(configRoot, "vilt"))
	run(t, chabBin, chabEnv, "--base-url", "http://127.0.0.1:9", "profile", "create", "isolated", "--plain")
	run(t, chabBin, chabEnv, "profile", "delete", "isolated", "--yes", "--no-prompt", "--plain")
	afterVILT := snapshotPath(t, filepath.Join(configRoot, "vilt"))
	if !bytes.Equal(beforeVILT, afterVILT) {
		t.Fatalf("VILT state changed after Chab profile create/delete\nbefore=%q\nafter=%q", beforeVILT, afterVILT)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root")
		}
		dir = parent
	}
}

func build(t *testing.T, dir, output, pkg string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", output, pkg)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s in %s failed: %v\n%s", pkg, dir, err, out)
	}
}

func commandOutput(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed in %s: %v\n%s", name, args, dir, err, out)
	}
	return string(out)
}

func run(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", bin, args, err, out)
	}
	return string(out)
}

func runWithInput(t *testing.T, bin string, env []string, input string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", bin, args, err, out)
	}
	return string(out)
}

func productEnv(configRoot string, pairs ...string) []string {
	env := make([]string, 0, len(os.Environ())+len(pairs)+1)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "CHAB_") || strings.HasPrefix(kv, "VILT_") || strings.HasPrefix(kv, "XDG_CONFIG_HOME=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "XDG_CONFIG_HOME="+configRoot)
	env = append(env, pairs...)
	return env
}

func chabWhoamiEnvelope() string {
	return `{
  "data": {
    "principal_type": "team",
    "team_id": 42,
    "token_id": "tok_iso_internal",
    "token_public_id": "tok_iso_public",
    "scopes": ["api:credits:read"],
    "token_controls": {
      "policy_revision": 1,
      "feature_access": {"mode": "full", "snapshot_stale": false},
      "ip_restrictions": {"restricted": false, "allow_rule_count": 0, "deny_rule_count": 0},
      "spending": {"mode": "enabled", "active_reserved_credits": 0, "allowance": null},
      "project_access": {"mode": "all", "selected_project_ids": [], "selected_count": 0}
    }
  },
  "request_id": "req-chab-iso"
}`
}

func viltWhoamiEnvelope() string {
	return `{
  "data": {
    "team": {"id": 42, "display_id": "team_01HY0000000000000000000000", "name": "VILT Team", "plan": "pro", "plan_label": "Pro", "api_access": true},
    "key": {
      "id": "ak_01HY0000000000000000000000",
      "display_id": "ak_01HY0000000000000000000000",
      "name": "Local CLI",
      "preset": "full_access",
      "expiration": {"state": "active", "expires_at": null},
      "last_used_at": "2026-05-19T10:15:30+00:00",
      "created_at": "2026-05-01T09:00:00+00:00"
    },
    "capabilities": [],
    "project_scope": {
      "mode": "all_projects",
      "selected_count": 0,
      "selected_projects": {"data": [], "has_more": false},
      "applies_to_team_level_groups": true
    }
  },
  "request_id": "req-vilt-iso"
}`
}

func snapshotPath(t *testing.T, path string) []byte {
	t.Helper()
	var buf bytes.Buffer
	err := filepath.WalkDir(path, func(p string, entry os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}
		buf.WriteString(filepath.ToSlash(rel))
		buf.WriteByte('\n')
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		buf.Write(data)
		buf.WriteByte('\n')
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("output missing %q:\n%s", want, got)
	}
}

func assertNotContains(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Fatalf("output unexpectedly contained %q:\n%s", want, got)
	}
}
