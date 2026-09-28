package cli_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

// updateHelpGolden is intentionally opt-in so ordinary test runs detect help
// drift instead of rewriting the reference files.
var updateHelpGolden = flag.Bool("update-help-golden", false, "rewrite help golden files")

// TestHelpGoldens pins representative help surfaces byte-for-byte. The set
// includes root, framework-owned, functional, alias, and command-family surfaces so
// global help-template changes are caught in one place.
func TestHelpGoldens(t *testing.T) {
	entries := cli.Catalog()
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "chab.golden", args: []string{"--help"}},
		{name: "chab-auth.golden", args: []string{"auth", "--help"}},
		{name: "chab-completion.golden", args: []string{"completion", "--help"}},
		{name: "chab-help.golden", args: []string{"help", "--help"}},
		{name: "chab-version.golden", args: []string{"version", "--help"}},
		{name: "chab-whoami.golden", args: []string{"whoami", "--help"}},
		{name: "chab-health.golden", args: []string{"health", "--help"}},
		{name: "chab-errors.golden", args: []string{"errors", "--help"}},
		{name: "chab-doctor.golden", args: []string{"doctor", "--help"}},
		{name: "chab-setup.golden", args: []string{"setup", "--help"}},
		{name: "chab-auth-env.golden", args: []string{"auth", "env", "--help"}},
		{name: "chab-auth-login.golden", args: []string{"auth", "login", "--help"}},
		{name: "chab-auth-status.golden", args: []string{"auth", "status", "--help"}},
		{name: "chab-auth-logout.golden", args: []string{"auth", "logout", "--help"}},
		{name: "chab-login.golden", args: []string{"login", "--help"}},
		{name: "chab-logout.golden", args: []string{"logout", "--help"}},
		{name: "chab-profile.golden", args: []string{"profile", "--help"}},
		{name: "chab-profile-list.golden", args: []string{"profile", "list", "--help"}},
		{name: "chab-profile-show.golden", args: []string{"profile", "show", "--help"}},
		{name: "chab-profile-create.golden", args: []string{"profile", "create", "--help"}},
		{name: "chab-profile-use.golden", args: []string{"profile", "use", "--help"}},
		{name: "chab-profile-delete.golden", args: []string{"profile", "delete", "--help"}},
		{name: "chab-config.golden", args: []string{"config", "--help"}},
		{name: "chab-config-path.golden", args: []string{"config", "path", "--help"}},
		{name: "chab-config-list.golden", args: []string{"config", "list", "--help"}},
		{name: "chab-config-get.golden", args: []string{"config", "get", "--help"}},
		{name: "chab-config-set.golden", args: []string{"config", "set", "--help"}},
		{name: "chab-api.golden", args: []string{"api", "--help"}},
		{name: "chab-api-get.golden", args: []string{"api", "get", "--help"}},
		{name: "chab-api-post.golden", args: []string{"api", "post", "--help"}},
		{name: "chab-api-patch.golden", args: []string{"api", "patch", "--help"}},
		{name: "chab-api-delete.golden", args: []string{"api", "delete", "--help"}},
		{name: "chab-operations.golden", args: []string{"operations", "--help"}},
		{name: "chab-operations-start.golden", args: []string{"operations", "start", "--help"}},
		{name: "chab-operations-actions-list.golden", args: []string{"operations", "actions", "list", "--help"}},
		{name: "chab-search-web.golden", args: []string{"search", "web", "--help"}},
		{name: "chab-llm-generate.golden", args: []string{"llm", "generate", "--help"}},
		{name: "chab-research-deep.golden", args: []string{"research", "deep", "--help"}},
		{name: "chab-credits.golden", args: []string{"credits", "--help"}},
		{name: "chab-credits-balance.golden", args: []string{"credits", "balance", "--help"}},
		{name: "chab-credits-transactions.golden", args: []string{"credits", "transactions", "--help"}},
		{name: "chab-mcp.golden", args: []string{"mcp", "--help"}},
		{name: "chab-mcp-serve.golden", args: []string{"mcp", "serve", "--help"}},
	} {
		t.Run(strings.TrimSuffix(test.name, ".golden"), func(t *testing.T) {
			got := renderHelpGolden(t, test.args...)
			assertHelpTextSafe(t, test.name+" rendered help", string(got), entries)
			path := filepath.Join("testdata", "help", test.name)
			if *updateHelpGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			assertHelpTextSafe(t, test.name+" golden", string(want), entries)
			if !bytes.Equal(got, want) {
				t.Fatalf("%s help drifted; run go test ./internal/cli -update-help-golden", test.name)
			}
		})
	}
}

// TestHelpSpellingsMatch checks Cobra's alternate help entrypoints so users get
// the same bytes from topic help, --help, and bare family help where supported.
func TestHelpSpellingsMatch(t *testing.T) {
	assertSameHelpBytes(t, []string{"help", "setup"}, []string{"setup", "--help"})
	assertSameHelpBytes(t, []string{"help", "auth", "env"}, []string{"auth", "env", "--help"})
	assertSameHelpBytes(t, []string{"help", "auth", "status"}, []string{"auth", "status", "--help"})
	assertSameHelpBytes(t, []string{"auth"}, []string{"auth", "--help"})
	assertSameHelpBytes(t, []string{"credits"}, []string{"credits", "--help"})
	assertSameHelpBytes(t, []string{"help", "credits"}, []string{"credits", "--help"})
	assertSameHelpBytes(t, []string{"help", "health"}, []string{"health", "--help"})
	assertSameHelpBytes(t, []string{"help", "errors"}, []string{"errors", "--help"})
	assertSameHelpBytes(t, []string{"profile"}, []string{"profile", "--help"})
	assertSameHelpBytes(t, []string{"help", "profile"}, []string{"profile", "--help"})
	assertSameHelpBytes(t, []string{"config"}, []string{"config", "--help"})
	assertSameHelpBytes(t, []string{"help", "config"}, []string{"config", "--help"})
	assertSameHelpBytes(t, []string{"api"}, []string{"api", "--help"})
	assertSameHelpBytes(t, []string{"help", "api"}, []string{"api", "--help"})
	assertSameHelpBytes(t, []string{"operations"}, []string{"operations", "--help"})
	assertSameHelpBytes(t, []string{"help", "operations"}, []string{"operations", "--help"})
	assertSameHelpBytes(t, []string{"mcp"}, []string{"mcp", "--help"})
	assertSameHelpBytes(t, []string{"help", "mcp"}, []string{"mcp", "--help"})
	assertSameHelpBytes(t, []string{"help", "mcp", "serve"}, []string{"mcp", "serve", "--help"})

	bare := renderCommandBytes(t)
	flagged := renderCommandBytes(t, "--help")
	topic := renderCommandBytes(t, "help")
	if !bytes.Equal(bare, flagged) || !bytes.Equal(bare, topic) {
		t.Fatalf("root bare, --help, and help output differ")
	}
}

func assertSameHelpBytes(t *testing.T, left, right []string) {
	t.Helper()
	leftBytes := renderCommandBytes(t, left...)
	rightBytes := renderCommandBytes(t, right...)
	if !bytes.Equal(leftBytes, rightBytes) {
		t.Fatalf("%v and %v help output differ", left, right)
	}
}

func renderHelpGolden(t *testing.T, args ...string) []byte {
	t.Helper()
	return renderCommandBytes(t, args...)
}

func renderCommandBytes(t *testing.T, args ...string) []byte {
	t.Helper()
	// Help goldens must not depend on a developer's profile, locale, terminal,
	// or auth state. The hermetic lookup makes accidental environment reads
	// visible as test failures elsewhere instead of changing the golden bytes.
	result := testutil.RunCommandWith(t, testutil.Options{LookupEnv: testutil.HermeticEnv(nil)}, args...)
	if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
		t.Fatalf("%v result = %#v", args, result)
	}
	return []byte(result.Stdout)
}
