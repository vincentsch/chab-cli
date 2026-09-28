package releasecheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/testutil"
)

// TestNoSecretSmokeClearsCanonicalViltEnv makes the shell script share the same
// env-clear contract as Go tests. This prevents a host-only CHAB_* variable from
// making offline smoke output depend on a developer machine.
func TestNoSecretSmokeClearsCanonicalViltEnv(t *testing.T) {
	repoRoot := findRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash("scripts/smoke-release-no-secret.sh")))
	if err != nil {
		t.Fatal(err)
	}
	got := parseClearList(t, string(data))
	want := testutil.ViltEnvNames()
	if len(got) != len(want) {
		t.Fatalf("no-secret smoke clears %v, want testutil.ViltEnvNames() %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("clear-list[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// parseClearList extracts the names from the `for name in <names>; do` line
// that follows the `# clear-chab-env` marker comment in the script.
func parseClearList(t *testing.T, script string) []string {
	t.Helper()
	lines := strings.Split(script, "\n")
	for i, ln := range lines {
		if strings.Contains(ln, "clear-chab-env") {
			for _, next := range lines[i+1:] {
				trimmed := strings.TrimSpace(next)
				if strings.HasPrefix(trimmed, "for name in ") {
					body := strings.TrimPrefix(trimmed, "for name in ")
					if idx := strings.Index(body, ";"); idx >= 0 {
						body = body[:idx]
					}
					return strings.Fields(body)
				}
			}
		}
	}
	t.Fatal("could not find clear-chab-env for-loop in no-secret smoke script")
	return nil
}
