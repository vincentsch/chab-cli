package manualdocs_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/manualdocs"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

// generatedTimestampPattern catches accidental build dates in generated docs.
// Static command docs should not change merely because generation ran today.
var generatedTimestampPattern = regexp.MustCompile(`\b20\d{2}-\d{2}-\d{2}(?:[T ][0-2]\d:[0-5]\d(?::[0-5]\d)?)?`)

const reservedFamilyManualSentence = "This command family is planned for a later release of chab and cannot be run yet."

func TestGenerateManualDocsFileSetAndSafety(t *testing.T) {
	root, entries := buildManualDocsTree(t)
	files := testutil.GenerateManualDocs(t, root, entries)

	got := sortedMapKeys(files)
	want := expectedManualDocPaths(entries)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("manual doc paths = %v, want %v", got, want)
	}
	assertManualDocsSafe(t, files, entries)
}

func TestGenerateManualDocsIsDeterministic(t *testing.T) {
	root, entries := buildManualDocsTree(t)
	first, err := manualdocs.Generate(root, entries)
	if err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	second, err := manualdocs.Generate(root, entries)
	if err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	if !byteMapsEqual(first, second) {
		t.Fatalf("manual docs changed across repeated generation on the same root")
	}

	// Cobra mutates command internals while rendering help and flag sets. A
	// fresh tree check catches hidden dependence on whether a command was
	// rendered earlier in the same process.
	freshRoot, freshEntries := buildManualDocsTree(t)
	third, err := manualdocs.Generate(freshRoot, freshEntries)
	if err != nil {
		t.Fatalf("fresh Generate() error = %v", err)
	}
	if !byteMapsEqual(first, third) {
		t.Fatalf("manual docs changed across fresh root generation")
	}
}

func TestSyntheticReservedManualFixtureOwnsPlannedRendering(t *testing.T) {
	root := &cobra.Command{Use: "chab", Long: "Synthetic command reference."}
	family := &cobra.Command{
		Use:   "reserved",
		Short: "Manage reserved behavior",
		Long:  "Manage a synthetic reserved family.\n\n" + cli.FamilyReservedSentence,
		Run:   func(*cobra.Command, []string) {},
	}
	leaf := &cobra.Command{
		Use:   "leaf",
		Short: "Run reserved behavior",
		Long:  "Run a synthetic reserved behavior.\n\n" + cli.LeafReservedSentence,
		Run:   func(*cobra.Command, []string) {},
	}
	family.AddCommand(leaf)
	root.AddCommand(family)
	entries := []cli.Entry{
		{
			Spec: cli.CommandSpec{Path: "reserved", Summary: family.Short},
			Sidecar: cli.Sidecar{
				Status:  cli.StatusReserved,
				DocPath: "manual/commands/chab-reserved.md",
			},
		},
		{
			Spec: cli.CommandSpec{Path: "reserved leaf", Summary: leaf.Short},
			Sidecar: cli.Sidecar{
				Status:  cli.StatusReserved,
				DocPath: "manual/commands/chab-reserved-leaf.md",
			},
		},
	}
	files, err := manualdocs.Generate(root, entries)
	if err != nil {
		t.Fatal(err)
	}
	if got := reservedChildren(entries, "reserved"); len(got) != 1 {
		t.Fatalf("synthetic reserved children = %d, want 1", len(got))
	}
	familyPage := string(files["manual/commands/chab-reserved.md"])
	for _, want := range []string{reservedFamilyManualSentence, "none (planned command)", "## Subcommands", "chab reserved leaf"} {
		if !strings.Contains(familyPage, want) {
			t.Fatalf("synthetic family manual missing %q:\n%s", want, familyPage)
		}
	}
	leafPage := string(files["manual/commands/chab-reserved-leaf.md"])
	for _, want := range []string{cli.LeafReservedSentence, "none (planned command)"} {
		if !strings.Contains(leafPage, want) {
			t.Fatalf("synthetic leaf manual missing %q:\n%s", want, leafPage)
		}
	}
	for path, page := range files {
		if strings.Contains(string(page), "## Examples") {
			t.Fatalf("%s synthetic reserved manual contains examples:\n%s", path, page)
		}
	}
}

func TestFunctionalManualPagesDoNotDescribePlannedCommands(t *testing.T) {
	root, entries := buildManualDocsTree(t)
	files := testutil.GenerateManualDocs(t, root, entries)

	for _, entry := range entries {
		if entry.Sidecar.Status == cli.StatusReserved {
			continue
		}
		page := string(files[entry.Sidecar.DocPath])
		for _, forbidden := range []string{
			"none (planned command)",
			cli.FamilyReservedSentence,
			cli.LeafReservedSentence,
		} {
			if strings.Contains(page, forbidden) {
				t.Fatalf("%s manual page contains reserved wording %q:\n%s", entry.Spec.Path, forbidden, page)
			}
		}
		if (entry.Spec.Path == "auth" || entry.Spec.Path == "api" || entry.Spec.Path == "credits" || entry.Spec.Path == "profile" || entry.Spec.Path == "config") && !strings.Contains(page, "none (command family)") {
			t.Fatalf("%s manual page does not describe command-family output modes:\n%s", entry.Spec.Path, page)
		}
	}
}

func TestRawAPIManualPagesMatchFunctionalBoundary(t *testing.T) {
	root, entries := buildManualDocsTree(t)
	files := testutil.GenerateManualDocs(t, root, entries)

	family := string(files["manual/commands/chab-api.md"])
	for _, want := range []string{"none (command family)", "Runs without requiring a credential", "chab api get", "chab api post", "chab api patch", "chab api delete"} {
		if !strings.Contains(family, want) {
			t.Fatalf("raw API family manual missing %q:\n%s", want, family)
		}
	}

	for _, path := range []string{
		"manual/commands/chab-api-get.md",
		"manual/commands/chab-api-post.md",
		"manual/commands/chab-api-patch.md",
		"manual/commands/chab-api-delete.md",
	} {
		page := string(files[path])
		for _, want := range []string{"human, JSON, plain, jq, template", "Requires a stored credential", "## Examples", "--raw", "--query", "--secret-field", "--include-meta", "## Metadata", "wrapped under `data`"} {
			if !strings.Contains(page, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, page)
			}
		}
		for _, forbidden := range []string{"none (planned command)", cli.LeafReservedSentence, "planned for a later release"} {
			if strings.Contains(page, forbidden) {
				t.Fatalf("%s contains inactive behavior %q:\n%s", path, forbidden, page)
			}
		}
		if !strings.HasSuffix(path, "get.md") {
			for _, want := range []string{"--body", "--body-file", "--field", "--dry-run", "--idempotency-key"} {
				if !strings.Contains(page, want) {
					t.Fatalf("%s missing %q:\n%s", path, want, page)
				}
			}
			confirmationPhrase := "does not ask for confirmation"
			if strings.HasSuffix(path, "post.md") {
				confirmationPhrase = "same confirmation rule as native commands"
			}
			if !strings.Contains(page, confirmationPhrase) {
				t.Fatalf("%s missing %q:\n%s", path, confirmationPhrase, page)
			}
		}
	}

	get := string(files["manual/commands/chab-api-get.md"])
	for _, want := range []string{"--all", "--limit", "--cursor", "--page-size"} {
		if !strings.Contains(get, want) {
			t.Fatalf("raw GET manual missing %q:\n%s", want, get)
		}
	}
}

func TestProfileAndConfigManualPagesMatchLocalBoundary(t *testing.T) {
	root, entries := buildManualDocsTree(t)
	files := testutil.GenerateManualDocs(t, root, entries)

	for _, path := range []string{"manual/commands/chab-profile.md", "manual/commands/chab-config.md"} {
		page := string(files[path])
		for _, want := range []string{"none (command family)", "Runs without requiring a credential", "## Examples"} {
			if !strings.Contains(page, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, page)
			}
		}
	}

	for _, path := range []string{
		"manual/commands/chab-profile-list.md",
		"manual/commands/chab-profile-show.md",
		"manual/commands/chab-config-path.md",
		"manual/commands/chab-config-list.md",
		"manual/commands/chab-config-get.md",
	} {
		page := string(files[path])
		for _, want := range []string{"human, JSON, plain, jq, template", "Runs without requiring a credential", "## Examples", "--template"} {
			if !strings.Contains(page, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, page)
			}
		}
		for _, forbidden := range []string{"--include-meta", "--dry-run", "none (planned command)", cli.LeafReservedSentence} {
			if strings.Contains(page, forbidden) {
				t.Fatalf("%s contains inactive behavior %q:\n%s", path, forbidden, page)
			}
		}
	}

	for _, path := range []string{
		"manual/commands/chab-profile-create.md",
		"manual/commands/chab-profile-use.md",
		"manual/commands/chab-profile-delete.md",
		"manual/commands/chab-config-set.md",
	} {
		page := string(files[path])
		for _, want := range []string{"human, plain", "Runs without requiring a credential", "## Examples"} {
			if !strings.Contains(page, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, page)
			}
		}
		for _, forbidden := range []string{"--include-meta", "--dry-run", "none (planned command)", cli.LeafReservedSentence} {
			if strings.Contains(page, forbidden) {
				t.Fatalf("%s contains inactive behavior %q:\n%s", path, forbidden, page)
			}
		}
	}
}

func TestAuthenticationEnvironmentManualMatchesLocalReportBoundary(t *testing.T) {
	root, entries := buildManualDocsTree(t)
	files := testutil.GenerateManualDocs(t, root, entries)

	family := string(files["manual/commands/chab-auth.md"])
	for _, want := range []string{
		"chab auth env",
		"chab auth login",
		"chab auth logout",
		"chab auth status",
	} {
		if !strings.Contains(family, want) {
			t.Fatalf("auth family manual missing %q:\n%s", want, family)
		}
	}

	page := string(files["manual/commands/chab-auth-env.md"])
	for _, want := range []string{
		"human, JSON, plain, jq, template",
		"Runs without requiring a credential",
		"api-key-setup",
		"--plain",
		"--json",
		"--jq",
		"--template",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("auth env manual missing %q:\n%s", want, page)
		}
	}
	for _, forbidden := range []string{"--include-meta", "--dry-run", "Requires a stored credential"} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("auth env manual contains unsupported behavior %q:\n%s", forbidden, page)
		}
	}
}

func TestMCPManualPagesDescribeMixedAuth(t *testing.T) {
	root, entries := buildManualDocsTree(t)
	files := testutil.GenerateManualDocs(t, root, entries)

	for _, path := range []string{"manual/commands/chab-mcp.md", "manual/commands/chab-mcp-serve.md"} {
		page := string(files[path])
		for _, want := range []string{
			"Runs without requiring a credential.",
			"Startup, discovery, `tools/list`, `chab_auth_env`, `chab_health`, and `chab_errors` do not require credentials.",
			"`chab_auth_me`",
			"`chab_credits_get`",
			"`chab_credits_transactions_list`",
			"`chab_search_web`",
			"`chab_action_resume`",
			"Changing `CHAB_API_KEY` in the host environment requires restarting or relaunching the MCP server process.",
		} {
			if !strings.Contains(page, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, page)
			}
		}
	}

	serve := string(files["manual/commands/chab-mcp-serve.md"])
	for _, want := range []string{
		"direct terminal execution waits for protocol frames",
		"chab mcp serve   # protocol testing only; hosts launch this",
	} {
		if !strings.Contains(serve, want) {
			t.Fatalf("mcp serve manual missing %q:\n%s", want, serve)
		}
	}
}

func TestManualDescriptionsPreserveIndentedBlocks(t *testing.T) {
	root, entries := buildManualDocsTree(t)
	files := testutil.GenerateManualDocs(t, root, entries)

	version := string(files["manual/commands/chab-version.md"])
	for _, want := range []string{
		"```text\nversion     release version",
		"go_version  Go runtime version",
	} {
		if !strings.Contains(version, want) {
			t.Fatalf("version manual page missing fenced field block %q:\n%s", want, version)
		}
	}

	index := string(files["manual/commands/README.md"])
	if !strings.Contains(index, "```text\nchab version --json") ||
		!strings.Contains(index, "chab health            # inspect public API health\n") ||
		!strings.Contains(index, "chab errors --json     # inspect public API error catalog\n") ||
		!strings.Contains(index, "chab api get /me --json # call a public API path directly\n") ||
		!strings.Contains(index, "chab mcp serve --help  # configure local MCP tools\n```") {
		t.Fatalf("manual index missing fenced quick-start block:\n%s", index)
	}
}

func TestSystemManualPagesMatchFunctionalBoundaries(t *testing.T) {
	root, entries := buildManualDocsTree(t)
	files := testutil.GenerateManualDocs(t, root, entries)

	for _, path := range []string{
		"manual/commands/chab-health.md",
		"manual/commands/chab-errors.md",
	} {
		page := string(files[path])
		for _, want := range []string{"human, JSON, plain, jq, template", "Runs without requiring a credential", "## Examples"} {
			if !strings.Contains(page, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, page)
			}
		}
		for _, forbidden := range []string{"Requires a stored credential", "none (planned command)", cli.LeafReservedSentence, "--include-meta", "## Metadata"} {
			if strings.Contains(page, forbidden) {
				t.Fatalf("%s advertises inactive behavior %q:\n%s", path, forbidden, page)
			}
		}
	}
}

func TestCreditsManualPagesMatchFunctionalBoundary(t *testing.T) {
	root, entries := buildManualDocsTree(t)
	files := testutil.GenerateManualDocs(t, root, entries)

	family := string(files["manual/commands/chab-credits.md"])
	for _, want := range []string{"Balance inspection and cursor-paginated transaction history", "none (command family)", "chab credits balance", "chab credits transactions"} {
		if !strings.Contains(family, want) {
			t.Fatalf("credits family manual missing %q:\n%s", want, family)
		}
	}
	for _, forbidden := range []string{reservedFamilyManualSentence, cli.FamilyReservedSentence, "none (planned command)"} {
		if strings.Contains(family, forbidden) {
			t.Fatalf("credits family manual contains reserved-family wording %q:\n%s", forbidden, family)
		}
	}

	balance := string(files["manual/commands/chab-credits-balance.md"])
	for _, want := range []string{"human, JSON, plain, jq, template", "Requires a stored credential", "## Examples", "chab credits balance --template", "--include-meta", "## Metadata", "wrapped under `data`"} {
		if !strings.Contains(balance, want) {
			t.Fatalf("credits balance manual missing %q:\n%s", want, balance)
		}
	}
	for _, forbidden := range []string{"response metadata", "none (planned command)", cli.LeafReservedSentence} {
		if strings.Contains(strings.ToLower(balance), strings.ToLower(forbidden)) {
			t.Fatalf("credits balance manual contains inactive behavior %q:\n%s", forbidden, balance)
		}
	}

	transactions := string(files["manual/commands/chab-credits-transactions.md"])
	for _, want := range []string{"human, JSON, plain, jq, template", "Requires a stored credential", "## Examples", "## Flags", "--include-meta", "## Metadata", "wrapped under `data`"} {
		if !strings.Contains(transactions, want) {
			t.Fatalf("credits transactions manual missing %q:\n%s", want, transactions)
		}
	}
	for _, flag := range []string{"--limit", "--all", "--cursor", "--page-size"} {
		if !strings.Contains(transactions, flag) {
			t.Fatalf("credits transactions manual missing active %s:\n%s", flag, transactions)
		}
	}
	for _, flag := range []string{"`--type`", "`--since`", "`--until`", "`--sort`", "`--page`", "`--per-page`"} {
		if strings.Contains(transactions, flag) {
			t.Fatalf("credits transactions manual contains unsupported %s:\n%s", flag, transactions)
		}
	}
	for _, forbidden := range []string{"none (planned command)", cli.LeafReservedSentence, "planned for a later release"} {
		if strings.Contains(transactions, forbidden) {
			t.Fatalf("credits transactions manual contains %q:\n%s", forbidden, transactions)
		}
	}
}

func TestMetadataManualSectionsMatchCatalogExactly(t *testing.T) {
	root, entries := buildManualDocsTree(t)
	files := testutil.GenerateManualDocs(t, root, entries)
	var got []string
	for _, entry := range entries {
		page := string(files[entry.Sidecar.DocPath])
		hasSection := strings.Contains(page, "## Metadata\n")
		if hasSection != entry.Spec.SupportsMeta {
			t.Fatalf("%s metadata section = %t, catalog support = %t:\n%s", entry.Spec.Path, hasSection, entry.Spec.SupportsMeta, page)
		}
		if hasSection {
			got = append(got, entry.Spec.Path)
			for _, want := range []string{"`--include-meta` is supported", "`--json`", "`--jq`", "`--template`", "wrapped under `data`", "under `meta`", "metadata"} {
				if !strings.Contains(page, want) {
					t.Fatalf("%s metadata section missing %q:\n%s", entry.Spec.Path, want, page)
				}
			}
		}
	}
	want := []string{
		"api delete",
		"api get",
		"api patch",
		"api post",
		"billing auto-recharge show",
		"billing auto-recharge update",
		"billing packages",
		"billing purchases create",
		"billing purchases show",
		"billing purchases wait",
		"billing reconciliation",
		"billing show",
		"credits balance",
		"credits transactions",
		"drive connections",
		"drive items download",
		"drive items list",
		"drive items show",
		"drive search",
		"files download",
		"files list",
		"files show",
		"files wait",
		"mail attachments download",
		"mail connections",
		"mail drafts show",
		"mail folders",
		"mail messages body",
		"mail personas",
		"mail search",
		"mail threads list",
		"mail threads show",
		"operations artifact",
		"operations artifact download",
		"operations list",
		"operations result",
		"operations show",
		"operations wait",
		"project archive",
		"project create",
		"project delete",
		"project list",
		"project pause",
		"project resume",
		"project show",
		"project update",
		"tokens approvals create",
		"tokens approvals wait",
		"tokens create",
		"tokens list",
		"tokens revoke",
		"tokens show",
		"tokens update",
		"usage",
		"webhooks deliveries list",
		"webhooks deliveries replay",
		"webhooks deliveries show",
		"webhooks endpoints create",
		"webhooks endpoints delete",
		"webhooks endpoints list",
		"webhooks endpoints rotate-secret",
		"webhooks endpoints show",
		"webhooks endpoints update",
		"webhooks replays create",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata manual paths = %v, want %v", got, want)
	}
}

func TestCompletionManualPageOwnsShellChildren(t *testing.T) {
	root, entries := buildManualDocsTree(t)
	files := testutil.GenerateManualDocs(t, root, entries)

	page := string(files["manual/commands/chab-completion.md"])
	for _, shell := range []string{"- bash", "- fish", "- powershell", "- zsh"} {
		if !strings.Contains(page, shell) {
			t.Fatalf("completion manual page missing shell %q:\n%s", shell, page)
		}
	}
	for path := range files {
		if strings.HasPrefix(path, "manual/commands/chab-completion-") {
			t.Fatalf("completion shell child generated separate page %s", path)
		}
	}
}

func TestManualDocsCheckDetectsDrift(t *testing.T) {
	root, entries := buildManualDocsTree(t)
	files := testutil.GenerateManualDocs(t, root, entries)

	t.Run("current", func(t *testing.T) {
		tmp := t.TempDir()
		if err := manualdocs.Write(tmp, files); err != nil {
			t.Fatal(err)
		}
		result, err := manualdocs.Check(tmp, files)
		if err != nil {
			t.Fatal(err)
		}
		if !result.OK() {
			t.Fatalf("current docs reported drift: %#v", result)
		}
	})

	t.Run("stale", func(t *testing.T) {
		tmp := t.TempDir()
		if err := manualdocs.Write(tmp, files); err != nil {
			t.Fatal(err)
		}
		stale := "manual/commands/chab-version.md"
		if err := os.WriteFile(filepath.Join(tmp, filepath.FromSlash(stale)), []byte("stale\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		result, err := manualdocs.Check(tmp, files)
		if err != nil {
			t.Fatal(err)
		}
		if result.OK() || !reflect.DeepEqual(result.Stale, []string{stale}) {
			t.Fatalf("stale drift = %#v", result)
		}
	})

	t.Run("missing", func(t *testing.T) {
		tmp := t.TempDir()
		if err := manualdocs.Write(tmp, files); err != nil {
			t.Fatal(err)
		}
		missing := "manual/commands/chab-version.md"
		if err := os.Remove(filepath.Join(tmp, filepath.FromSlash(missing))); err != nil {
			t.Fatal(err)
		}
		result, err := manualdocs.Check(tmp, files)
		if err != nil {
			t.Fatal(err)
		}
		if result.OK() || !reflect.DeepEqual(result.Missing, []string{missing}) {
			t.Fatalf("missing drift = %#v", result)
		}
	})

	t.Run("orphaned", func(t *testing.T) {
		tmp := t.TempDir()
		if err := manualdocs.Write(tmp, files); err != nil {
			t.Fatal(err)
		}
		orphaned := "manual/commands/chab-orphan.md"
		if err := os.WriteFile(filepath.Join(tmp, filepath.FromSlash(orphaned)), []byte("orphaned\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		result, err := manualdocs.Check(tmp, files)
		if err != nil {
			t.Fatal(err)
		}
		if result.OK() || !reflect.DeepEqual(result.Orphaned, []string{orphaned}) {
			t.Fatalf("orphaned drift = %#v", result)
		}
	})
}

func buildManualDocsTree(t *testing.T) (*cobra.Command, []cli.Entry) {
	t.Helper()
	root, entries, err := testutil.BuildTree(io.Discard, io.Discard, testutil.Options{
		LookupEnv:        testutil.HermeticEnv(nil),
		StdinIsTerminal:  func() bool { return false },
		StdoutIsTerminal: func() bool { return false },
	})
	if err != nil {
		t.Fatalf("BuildTree() error = %v", err)
	}
	return root, entries
}

func expectedManualDocPaths(entries []cli.Entry) []string {
	paths := make([]string, 0, len(entries)+1)
	for _, entry := range entries {
		paths = append(paths, entry.Sidecar.DocPath)
	}
	paths = append(paths, "manual/commands/README.md")
	sort.Strings(paths)
	return paths
}

func sortedMapKeys(files map[string][]byte) []string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func byteMapsEqual(left, right map[string][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for path, leftData := range left {
		if !bytes.Equal(leftData, right[path]) {
			return false
		}
	}
	return true
}

func reservedChildren(entries []cli.Entry, familyPath string) []cli.Entry {
	var out []cli.Entry
	prefix := familyPath + " "
	for _, entry := range entries {
		if entry.Sidecar.Status == cli.StatusReserved && strings.HasPrefix(entry.Spec.Path, prefix) {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Spec.Path < out[j].Spec.Path
	})
	return out
}

// assertManualDocsSafe applies the same public-surface hygiene rules to
// generated pages that command help and hand-written docs use.
func assertManualDocsSafe(t *testing.T, files map[string][]byte, entries []cli.Entry) {
	t.Helper()
	for path, data := range files {
		text := string(data)
		if !bytes.HasSuffix(data, []byte("\n")) || bytes.HasSuffix(data, []byte("\n\n")) {
			t.Fatalf("%s does not have exactly one trailing newline", path)
		}
		if bytes.Contains(data, []byte("\r")) {
			t.Fatalf("%s contains CR line endings", path)
		}
		timestampAudit := text
		if path == "manual/commands/chab-credits-transactions.md" {
			// This active command documents fixed RFC3339 examples. Remove those
			// contract literals before checking for accidental build timestamps.
			for _, literal := range []string{
				"2026-05-19T10:15:30Z",
				"2026-05-19T12:15:30+02:00",
				"2026-05-01T00:00:00Z",
			} {
				timestampAudit = strings.ReplaceAll(timestampAudit, literal, "")
			}
		}
		if generatedTimestampPattern.MatchString(timestampAudit) {
			t.Fatalf("%s contains generated date or timestamp:\n%s", path, text)
		}
		testutil.AssertSafe(t, path, text)
		if testutil.TicketIDPattern.MatchString(text) {
			t.Fatalf("%s contains planning ticket id:\n%s", path, text)
		}
		for _, entry := range entries {
			if entry.Sidecar.Owner != "" && strings.Contains(text, entry.Sidecar.Owner) {
				t.Fatalf("%s exposes catalog owner %q:\n%s", path, entry.Sidecar.Owner, text)
			}
		}
	}
}
