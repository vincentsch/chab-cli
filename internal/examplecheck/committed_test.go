package examplecheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCommittedStaticChecksAndExactScriptSet(t *testing.T) {
	repoRoot := findRepoRoot(t)
	manifest, errs, err := committedStaticChecks(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 0 {
		t.Fatalf("committed static errors:\n%s", joinErrors(errs))
	}
	got := manifestScripts(manifest)
	want := []string{
		"examples/agents/idempotent-raw-mutation.sh",
		"examples/ci/env-auth-and-errors.sh",
		"examples/support/profile-metadata.sh",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("manifest scripts = %#v, want %#v", got, want)
	}
	discovered, err := DiscoverScripts(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(discovered, "\n") != strings.Join(want, "\n") {
		t.Fatalf("discovered scripts = %#v, want %#v", discovered, want)
	}
}

func TestCommittedStaticChecksUsesProductionSafetyPath(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", []byte("module example.test/check\n\ngo 1.22\n"), 0o644)
	script := "examples/ci/safe.sh"
	token := strings.Join([]string{"sample", "credential"}, "|")
	writeTestFile(t, root, script, []byte("#!/bin/sh\n# "+token+"\n"), 0o755)
	manifest := `{
  "examples": [{
    "script": "examples/ci/safe.sh",
    "description": "safety fixture",
    "exit": 0,
    "stdout_contains": ["ok"],
    "mock_requests": 0,
    "commands": [{"path": ["api", "get"], "flags": ["json"]}]
  }]
}`
	writeTestFile(t, root, manifestPath, []byte(manifest), 0o644)
	_, errs, err := committedStaticChecks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := joinErrors(errs); !strings.Contains(got, "leaks key-shaped token") {
		t.Fatalf("static errors = %q", got)
	}
}

func TestCommittedScriptModeValidationIsLinuxOnly(t *testing.T) {
	root := t.TempDir()
	script := "examples/ci/not-executable.sh"
	writeTestFile(t, root, script, []byte("#!/bin/sh\n"), 0o644)

	for _, goos := range []string{"windows", "darwin"} {
		if errs := committedScriptModeErrors(root, []string{script}, goos); len(errs) != 0 {
			t.Fatalf("%s mode errors = %#v, want none", goos, errs)
		}
	}
	got := joinErrors(committedScriptModeErrors(root, []string{script}, "linux"))
	if !strings.Contains(got, "script is not executable") {
		t.Fatalf("Linux mode errors = %q", got)
	}
}

func TestCommittedCIExampleAllowsDefaultConfigAndAuthPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("committed automation example requires POSIX sh")
	}
	repoRoot := findRepoRoot(t)
	fakeBin := filepath.Join(t.TempDir(), "chab")
	writeTestFile(t, filepath.Dir(fakeBin), filepath.Base(fakeBin), []byte(`#!/bin/sh
set -eu
if [ "$#" -eq 3 ] && [ "$1" = auth ] && [ "$2" = env ] && [ "$3" = --json ]; then
  printf '{"profile":"local","base_url":"%s","api_base_url":"%s/v1","locale":"","secret_variable":"CHAB_API_KEY"}\n' "$CHAB_BASE_URL" "$CHAB_BASE_URL"
  exit 0
fi
if [ "$1" = api ] && [ "$2" = get ] && [ "$3" = /credits ]; then
  if [ "${4:-}" = --jq ]; then
    printf '%s\n' '131'
  else
    printf '%s\n' '{"spendable_balance": 131}'
  fi
  exit 0
fi
if [ "$1" = api ] && [ "$2" = get ] && [ "$3" = /missing ]; then
  printf '%s\n' '{"error":{"code":"not_found"}}' >&2
  exit 5
fi
exit 90
`), 0o755)

	command := exec.Command("sh", filepath.Join(repoRoot, "examples/ci/env-auth-and-errors.sh"))
	command.Dir = repoRoot
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"LC_ALL=C",
		"CHAB_BIN=" + fakeBin,
		"CHAB_API_KEY=example-value",
		"CHAB_BASE_URL=http://127.0.0.1:9",
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("CI example failed with documented environment: %v\n%s", err, output)
	}
	for _, want := range []string{
		`"profile":"local"`,
		`"base_url":"http://127.0.0.1:9"`,
		`"api_base_url":"http://127.0.0.1:9/v1"`,
		`"locale":""`,
		`"secret_variable":"CHAB_API_KEY"`,
		`"spendable_balance"`,
		`"code":"not_found"`,
		"raw API not-found exit verified",
	} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("CI example output missing %q:\n%s", want, output)
		}
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
			t.Fatal("could not locate repository root")
		}
		dir = parent
	}
}
