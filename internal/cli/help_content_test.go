package cli_test

import (
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestInitialPersistentFlags(t *testing.T) {
	root := cli.NewRootCommand(io.Discard, io.Discard)
	got := persistentFlagNames(root)
	want := []string{
		"api-base-url",
		"auth-file",
		"base-url",
		"config",
		"debug",
		"include-meta",
		"jq",
		"json",
		"locale",
		"no-ansi",
		"no-color",
		"no-pager",
		"no-prompt",
		"plain",
		"profile",
		"template",
		"yes",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("persistent flags = %v, want %v", got, want)
	}

	for _, flag := range []string{"dry-run", "quiet"} {
		result := testutil.RunCommand(t, "--"+flag)
		if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
			t.Fatalf("--%s result = %#v", flag, result)
		}
		if !strings.Contains(result.Stderr, "unknown flag: --"+flag) {
			t.Fatalf("--%s stderr missing unknown flag:\n%s", flag, result.Stderr)
		}
	}
}

func TestGlobalFlagUsageStatesCommandDependentSupport(t *testing.T) {
	root := cli.NewRootCommand(io.Discard, io.Discard)
	want := map[string][]string{
		"json":         {"where supported"},
		"include-meta": {"include safe response metadata in machine output where supported"},
		"jq":           {"supported JSON output"},
		"template":     {"supported JSON output"},
		"plain":        {"where supported"},
		"no-color":     {"where supported"},
		"no-ansi":      {"where supported"},
		"no-pager":     {"where supported"},
		"yes":          {"supported confirmations", "does not supply missing input"},
	}
	for name, substrings := range want {
		flag := root.PersistentFlags().Lookup(name)
		if flag == nil {
			t.Fatalf("missing persistent flag --%s", name)
		}
		for _, substring := range substrings {
			if !strings.Contains(flag.Usage, substring) {
				t.Fatalf("--%s usage %q missing %q", name, flag.Usage, substring)
			}
		}
	}
}

func TestHelpContentSafetyAndStructure(t *testing.T) {
	opts := testutil.Options{LookupEnv: testutil.HermeticEnv(nil)}
	root, entries, err := testutil.BuildTree(io.Discard, io.Discard, opts)
	if err != nil {
		t.Fatalf("BuildTree() error = %v", err)
	}
	entryByPath := entriesByPath(entries)

	// Scan both rendered help and source help. Rendered help catches Cobra
	// template output, while source help keeps structural checks away from
	// inherited persistent flags that Cobra repeats on every command.
	rootHelp := testutil.RunCommandWith(t, opts, "--help")
	testutil.AssertSuccess(t, rootHelp)
	assertHelpTextSafe(t, "root rendered help", rootHelp.Stdout, entries)
	assertHelpTextSafe(t, "root source help", root.Short+"\n"+root.Long, entries)
	assertNoPlaceholderText(t, "root source help", root.Short+"\n"+root.Long)

	for _, node := range cli.VisibleCommands(root, true) {
		args := append(strings.Fields(node.Path), "--help")
		result := testutil.RunCommandWith(t, opts, args...)
		testutil.AssertSuccess(t, result)
		assertHelpTextSafe(t, node.Path+" rendered help", result.Stdout, entries)
		assertHelpTextSafe(t, node.Path+" source help", sourceHelpText(node.Command), entries)
		assertCommandHelpStructure(t, node, entryByPath)
	}
}

func TestMCPServeHelpWarnsAgainstDirectTerminalExecution(t *testing.T) {
	root := cli.NewRootCommand(io.Discard, io.Discard)
	cmd, remaining, err := root.Find([]string{"mcp", "serve"})
	if err != nil || cmd == nil || len(remaining) != 0 {
		t.Fatalf("Find(mcp serve) = %v, %v, %v", cmd, remaining, err)
	}
	for _, want := range []string{
		"direct terminal execution waits for protocol frames",
		"Changing `CHAB_API_KEY` in the host environment requires restarting or relaunching the MCP server process.",
	} {
		if !strings.Contains(cmd.Long, want) {
			t.Fatalf("mcp serve Long missing %q:\n%s", want, cmd.Long)
		}
	}
	const example = "chab mcp serve   # protocol testing only; hosts launch this"
	if !strings.Contains(cmd.Example, example) {
		t.Fatalf("mcp serve examples missing %q:\n%s", example, cmd.Example)
	}
}

// assertCommandHelpStructure checks the command-facing prose promises that must
// stay aligned with catalog metadata: examples, related commands, reserved
// wording, and output-mode claims.
func assertCommandHelpStructure(t *testing.T, node cli.CommandNode, entryByPath map[string]cli.Entry) {
	t.Helper()
	cmd := node.Command
	if strings.TrimSpace(cmd.Use) == "" {
		t.Fatalf("%s has empty Use", node.Path)
	}
	if strings.TrimSpace(cmd.Short) == "" {
		t.Fatalf("%s has empty Short", node.Path)
	}

	entry, hasEntry := entryByPath[node.Path]
	exempt := helpContentExempt(node.Path)
	family := hasAvailableChildren(cmd)
	if (family || (hasEntry && entry.Sidecar.Status != cli.StatusReserved && !exempt)) && strings.TrimSpace(cmd.Long) == "" {
		t.Fatalf("%s has empty Long", node.Path)
	}
	assertNoPlaceholderText(t, node.Path+" source help", sourceHelpText(cmd))

	if hasEntry && entry.Sidecar.Status == cli.StatusReserved {
		if strings.TrimSpace(cmd.Example) != "" {
			t.Fatalf("%s reserved command has examples:\n%s", node.Path, cmd.Example)
		}
		if family {
			if !strings.Contains(cmd.Long, cli.FamilyReservedSentence) {
				t.Fatalf("%s family help missing reserved sentence:\n%s", node.Path, cmd.Long)
			}
		} else if !strings.Contains(cmd.Long, cli.LeafReservedSentence) {
			t.Fatalf("%s leaf help missing reserved sentence:\n%s", node.Path, cmd.Long)
		}
	}

	if !exempt && !strings.Contains(cmd.Long, "Related commands:") {
		t.Fatalf("%s help missing related-command guidance:\n%s", node.Path, cmd.Long)
	}

	if hasEntry && entry.Sidecar.Status != cli.StatusReserved && !family && !exempt {
		assertExampleInvokesPath(t, node.Path, cmd.Example, entryByPath)
	}
	if hasEntry {
		assertOutputModeHonesty(t, node.Path, entry, cmd, entryByPath)
	}
}

// assertOutputModeHonesty makes examples prove the modes a command claims. It
// resolves each example command before checking flags because examples may point
// at aliases or sibling commands instead of the page being linted.
func assertOutputModeHonesty(t *testing.T, path string, entry cli.Entry, cmd *cobra.Command, entryByPath map[string]cli.Entry) {
	t.Helper()
	if entry.Sidecar.FrameworkOwned || entry.Sidecar.Status == cli.StatusReserved {
		return
	}
	modes := modeSet(entry)
	source := sourceHelpText(cmd)
	if modes["json"] && !strings.Contains(strings.ToLower(source), "json") {
		t.Fatalf("%s claims JSON but help does not mention JSON:\n%s", path, source)
	}
	if modes["plain"] && !strings.Contains(source, "--plain") {
		t.Fatalf("%s claims plain but help does not document --plain:\n%s", path, source)
	}
	if modes["jq"] && !exampleUsesFlag(cmd.Example, "--jq") {
		t.Fatalf("%s claims jq but examples do not show --jq:\n%s", path, cmd.Example)
	}
	if modes["template"] && !exampleUsesFlag(cmd.Example, "--template") {
		t.Fatalf("%s claims template but examples do not show --template:\n%s", path, cmd.Example)
	}
	for _, line := range exampleCommandLines(cmd.Example) {
		invokedPath, ok := resolveExamplePath(line, entryByPath)
		if !ok {
			continue
		}
		invokedEntry := entryByPath[invokedPath]
		invokedModes := modeSet(invokedEntry)
		for flag, mode := range map[string]string{
			"--json":     "json",
			"--plain":    "plain",
			"--jq":       "jq",
			"--template": "template",
		} {
			if commandLineUsesFlag(line, flag) && !invokedModes[mode] {
				t.Fatalf("%s example uses %s for %s, which does not claim %s output:\n%s", path, flag, invokedPath, mode, line)
			}
		}
	}
}

// assertExampleInvokesPath ensures executable leaf help includes at least one
// copyable example for the exact command path, not only nearby commands.
func assertExampleInvokesPath(t *testing.T, path, examples string, entryByPath map[string]cli.Entry) {
	t.Helper()
	for _, line := range exampleCommandLines(examples) {
		if invokedPath, ok := resolveExamplePath(line, entryByPath); ok && invokedPath == path {
			return
		}
	}
	t.Fatalf("%s examples do not invoke their own command path:\n%s", path, examples)
}

func assertHelpTextSafe(t *testing.T, label, text string, entries []cli.Entry) {
	t.Helper()
	testutil.AssertSafe(t, label, text)
	assertNoCatalogOwners(t, label, text, entries)
}

func assertNoCatalogOwners(t *testing.T, label, text string, entries []cli.Entry) {
	t.Helper()
	for _, entry := range entries {
		if entry.Sidecar.Owner != "" && strings.Contains(text, entry.Sidecar.Owner) {
			t.Fatalf("%s exposes catalog owner %q:\n%s", label, entry.Sidecar.Owner, text)
		}
	}
}

func assertNoPlaceholderText(t *testing.T, label, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, placeholder := range []string{"todo", "tbd", "placeholder", "lorem", "coming soon"} {
		if strings.Contains(lower, placeholder) {
			t.Fatalf("%s contains placeholder wording %q:\n%s", label, placeholder, text)
		}
	}
}

func sourceHelpText(cmd *cobra.Command) string {
	return strings.TrimSpace(cmd.Short + "\n" + cmd.Long + "\n" + cmd.Example)
}

func helpContentExempt(path string) bool {
	return path == "help" || path == "completion" || strings.HasPrefix(path, "completion ")
}

func hasAvailableChildren(cmd *cobra.Command) bool {
	for _, child := range cmd.Commands() {
		if child.IsAvailableCommand() {
			return true
		}
	}
	return false
}

func entriesByPath(entries []cli.Entry) map[string]cli.Entry {
	out := make(map[string]cli.Entry, len(entries))
	for _, entry := range entries {
		out[entry.Spec.Path] = entry
	}
	return out
}

func exampleCommandLines(examples string) []string {
	var lines []string
	for _, line := range strings.Split(examples, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "chab ") {
			lines = append(lines, line)
		}
	}
	return lines
}

// resolveExamplePath chooses the longest matching catalog path so nested
// commands such as "auth status" are not mistaken for their parent family.
func resolveExamplePath(line string, entryByPath map[string]cli.Entry) (string, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 || fields[0] != "chab" {
		return "", false
	}
	paths := make([]string, 0, len(entryByPath))
	for path := range entryByPath {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		left := len(strings.Fields(paths[i]))
		right := len(strings.Fields(paths[j]))
		if left == right {
			return paths[i] < paths[j]
		}
		return left > right
	})
	args := fields[1:]
	for _, path := range paths {
		pathFields := strings.Fields(path)
		if len(pathFields) > len(args) {
			continue
		}
		matches := true
		for i, field := range pathFields {
			if args[i] != field {
				matches = false
				break
			}
		}
		if matches {
			return path, true
		}
	}
	return "", false
}

func exampleUsesFlag(examples, flag string) bool {
	for _, line := range exampleCommandLines(examples) {
		if commandLineUsesFlag(line, flag) {
			return true
		}
	}
	return false
}

func commandLineUsesFlag(line, flag string) bool {
	for _, field := range strings.Fields(line) {
		if field == flag || strings.HasPrefix(field, flag+"=") {
			return true
		}
	}
	return false
}

func persistentFlagNames(root *cobra.Command) []string {
	var names []string
	root.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
		names = append(names, flag.Name)
	})
	sort.Strings(names)
	return names
}
