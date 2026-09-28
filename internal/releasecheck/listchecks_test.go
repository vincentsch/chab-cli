package releasecheck

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

// The fixed no-auth release-verification subset. root is the bare Cobra root;
// the rest must remain no-auth catalog paths.
var expectedListChecks = []string{"root", "help", "version", "config", "credits", "doctor", "health", "errors", "mcp", "completion", "config path", "auth env"}
var catalogBackedChecks = []string{"help", "version", "config", "credits", "doctor", "health", "errors", "mcp", "completion", "config path", "auth env"}

// TestNoSecretSmokeListChecksMatchesCatalog keeps the shell script's public
// coverage list aligned with the production command catalog. The list is fixed,
// not derived from the catalog, so this test catches accidental drift.
func TestNoSecretSmokeListChecksMatchesCatalog(t *testing.T) {
	// Catalog half: active on every OS, including Windows.
	catalog := cli.Catalog()
	for _, path := range catalogBackedChecks {
		spec, ok := testutil.SpecByPath(catalog, path)
		if !ok {
			t.Fatalf("catalog missing no-auth release check %q", path)
		}
		if spec.Spec.RequiresAuth {
			t.Fatalf("release check %q is RequiresAuth: true; not a valid no-auth check", path)
		}
	}
	if _, ok := testutil.SpecByPath(catalog, "root"); ok {
		t.Fatal(`"root" must not be a catalog path; it is the bare Cobra root`)
	}
	if root := cli.NewRootCommand(io.Discard, io.Discard); root == nil || root.Name() != "chab" {
		t.Fatal("bare root command did not build as chab")
	}

	// Shell half: skipped on Windows; catalog half above still ran.
	skipWindows(t)
	res := runScript(t, "scripts/smoke-release-no-secret.sh", nil, "--list-checks")
	if res.exitCode != 0 {
		t.Fatalf("--list-checks exit %d; stderr=%s", res.exitCode, res.stderr)
	}
	// Compare line-by-line: 'config path' is one line containing a space.
	got := strings.Split(strings.TrimRight(res.stdout, "\n"), "\n")
	if len(got) != len(expectedListChecks) {
		t.Fatalf("--list-checks lines = %#v, want %#v", got, expectedListChecks)
	}
	for i := range expectedListChecks {
		if got[i] != expectedListChecks[i] {
			t.Fatalf("--list-checks line %d = %q, want %q", i, got[i], expectedListChecks[i])
		}
	}
}

func TestNoSecretSmokeExecutesCurrentShell(t *testing.T) {
	skipWindows(t)
	// The list contract is not enough: build a real binary and run the smoke so
	// command exits, stdout, and stderr stay aligned with the released shell.
	bin := buildViltBinary(t)
	res := runScript(t, "scripts/smoke-release-no-secret.sh", cleanEnv(nil), "--bin", bin)
	if res.exitCode != 0 {
		t.Fatalf("no-secret smoke exit %d; stdout=%s stderr=%s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "no-secret release smoke passed") {
		t.Fatalf("no-secret smoke stderr = %q, want pass marker", res.stderr)
	}
	if strings.TrimSpace(res.stdout) != "" {
		t.Fatalf("no-secret smoke stdout = %q, want empty", res.stdout)
	}
}

// buildViltBinary avoids `go run` for smoke coverage because the Go tool adds
// its own "exit status" line when the child exits nonzero. The release smoke
// needs the real binary's stdout and stderr.
func buildViltBinary(t *testing.T) string {
	t.Helper()
	repoRoot := findRepoRoot(t)
	bin := filepath.Join(t.TempDir(), "chab")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/chab")
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build chab binary: %v\n%s", err, stderr.String())
	}
	return bin
}
