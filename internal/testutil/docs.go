package testutil

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/manualdocs"
)

// RunHelp executes <path...> --help through RunCommandWith, asserts success,
// runs the shared safety lint, and returns stdout.
func RunHelp(t *testing.T, opts Options, path ...string) string {
	t.Helper()
	args := append([]string(nil), path...)
	args = append(args, "--help")
	result := RunCommandWith(t, opts, args...)
	AssertSuccess(t, result)
	AssertSafe(t, "help output", result.Stdout)
	return result.Stdout
}

// SpecByPath finds one catalog row in a per-tree catalog.
func SpecByPath(entries []cli.Entry, path string) (cli.Entry, bool) {
	for _, entry := range entries {
		if entry.Spec.Path == path {
			return entry, true
		}
	}
	return cli.Entry{}, false
}

// GenerateManualDocs renders manual pages in memory and runs the shared safety
// lint over every page. It never writes to the repository.
func GenerateManualDocs(t *testing.T, root *cobra.Command, entries []cli.Entry) map[string][]byte {
	t.Helper()
	files, err := manualdocs.Generate(root, entries)
	if err != nil {
		t.Fatalf("manualdocs.Generate() error = %v", err)
	}
	for path, data := range files {
		AssertSafe(t, path, string(bytes.TrimSpace(data)))
	}
	return files
}
