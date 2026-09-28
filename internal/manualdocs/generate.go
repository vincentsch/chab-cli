package manualdocs

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vincentsch/chab-cli/internal/cli"
)

const indexPath = "manual/commands/README.md"

const reservedFamilyManualSentence = "This command family is planned for a later release of chab and cannot be run yet."

// Generate renders one command-reference page per catalog entry plus the index.
func Generate(root *cobra.Command, entries []cli.Entry) (map[string][]byte, error) {
	cmds := map[string]*cobra.Command{}
	for _, node := range cli.VisibleCommands(root, false) {
		cmds[node.Path] = node.Command
	}

	// The manual reference is only trustworthy if the command tree and catalog
	// describe the same public surface. Check both directions before rendering
	// any pages so a missing row cannot silently disappear from the docs.
	entryByPath := make(map[string]cli.Entry, len(entries))
	for _, entry := range entries {
		entryByPath[entry.Spec.Path] = entry
		if _, ok := cmds[entry.Spec.Path]; !ok {
			return nil, fmt.Errorf("catalog entry %q does not resolve to a visible command", entry.Spec.Path)
		}
	}
	for path := range cmds {
		if _, ok := entryByPath[path]; !ok {
			return nil, fmt.Errorf("visible command %q has no catalog entry", path)
		}
	}

	files := make(map[string][]byte, len(entries)+1)
	for _, entry := range entries {
		files[entry.Sidecar.DocPath] = []byte(renderPage(entry, cmds[entry.Spec.Path], entryByPath))
	}
	files[indexPath] = []byte(renderIndex(root, entries))
	return files, nil
}

// renderPage converts one Cobra command plus its catalog row into a compact
// Markdown page. It keeps command help as the source of human prose while using
// catalog metadata for machine-checkable claims such as output modes and auth.
func renderPage(entry cli.Entry, cmd *cobra.Command, entryByPath map[string]cli.Entry) string {
	// LocalFlags has a Cobra side effect: it merges inherited persistent flags
	// into the command's flag set, which changes whether UseLine appends
	// "[flags]". Force that merge before reading UseLine so fresh and reused
	// roots render the same docs.
	localFlags := cmd.LocalFlags()
	usage := cmd.UseLine()

	var b strings.Builder
	writeHeading(&b, "# "+cmd.CommandPath())
	writeSection(&b, "## Usage", fencedText(usage))

	description := strings.TrimSpace(cmd.Long)
	if description == "" {
		description = strings.TrimSpace(cmd.Short)
	}
	if description != "" {
		writeSection(&b, "## Description", renderDescription(description, entry, cmd, entryByPath))
	}
	if subcommands := subcommandLines(cmd, entryByPath); len(subcommands) > 0 {
		writeSection(&b, "## Subcommands", strings.Join(subcommands, "\n"))
	}
	if strings.TrimSpace(cmd.Example) != "" {
		writeSection(&b, "## Examples", fencedText(cmd.Example))
	}
	if flags := localFlagLines(cmd, localFlags); len(flags) > 0 {
		writeSection(&b, "## Flags", strings.Join(flags, "\n"))
	}
	writeSection(&b, "## Output modes", outputModesText(entry))
	if entry.Spec.SupportsMeta {
		writeSection(&b, "## Metadata", "`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.")
	}
	writeSection(&b, "## Authentication", authenticationText(entry))
	if len(entry.Sidecar.HelpRequirements) > 0 {
		reqs := make([]string, 0, len(entry.Sidecar.HelpRequirements))
		for _, req := range entry.Sidecar.HelpRequirements {
			reqs = append(reqs, string(req))
		}
		writeSection(&b, "## Help topics", strings.Join(reqs, ", "))
	}
	if entry.Spec.Path == "completion" {
		writeSection(&b, "## Shells", strings.Join(completionShells(cmd), "\n"))
	}
	return singleTrailingNewline(b.String())
}

// renderIndex builds the generated command index from the root help and the
// catalog. Per-command pages document local flags; the index documents global
// persistent flags once so they are not repeated on every page.
func renderIndex(root *cobra.Command, entries []cli.Entry) string {
	var b strings.Builder
	writeHeading(&b, "# chab command reference")
	writeParagraph(&b, preserveIndentedBlocks(strings.TrimSpace(root.Long)))

	var commands []string
	for _, entry := range entries {
		commands = append(commands, fmt.Sprintf("- [chab %s](%s) - %s", entry.Spec.Path, path.Base(entry.Sidecar.DocPath), entry.Spec.Summary))
	}
	writeSection(&b, "## Commands", strings.Join(commands, "\n"))

	var flags []string
	root.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
		flags = append(flags, fmt.Sprintf("- `--%s` - %s", flag.Name, flag.Usage))
	})
	writeSection(&b, "## Global flags", strings.Join(flags, "\n"))
	return singleTrailingNewline(b.String())
}

func writeHeading(b *strings.Builder, text string) {
	b.WriteString(text)
	b.WriteString("\n\n")
}

func writeSection(b *strings.Builder, heading, body string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return
	}
	b.WriteString(heading)
	b.WriteString("\n\n")
	b.WriteString(body)
	b.WriteString("\n\n")
}

func writeParagraph(b *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	b.WriteString(text)
	b.WriteString("\n\n")
}

func fencedText(text string) string {
	return "```text\n" + strings.TrimRight(text, "\n") + "\n```"
}

func singleTrailingNewline(text string) string {
	return strings.TrimRight(text, "\n") + "\n"
}

// linkifyRelatedCommands turns the plain related-command block from Cobra help
// into Markdown links. It deliberately leaves all other prose unchanged so the
// manual page does not become a second, divergent copy of command help.
func linkifyRelatedCommands(text string, entryByPath map[string]cli.Entry) string {
	var out []string
	inRelated := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "Related commands:" {
			inRelated = true
			out = append(out, line)
			continue
		}
		if inRelated {
			if trimmed == "" {
				// A blank line ends the related-command block. Later prose, such
				// as alias notes, should stay exactly as command help wrote it.
				inRelated = false
				out = append(out, line)
				continue
			}
			if commandPath, ok := strings.CutPrefix(trimmed, "chab "); ok {
				if entry, ok := entryByPath[commandPath]; ok {
					out = append(out, fmt.Sprintf("- [chab %s](%s)", commandPath, path.Base(entry.Sidecar.DocPath)))
					continue
				}
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func renderDescription(text string, entry cli.Entry, cmd *cobra.Command, entryByPath map[string]cli.Entry) string {
	if entry.Sidecar.Status == cli.StatusReserved && hasAvailableSubcommands(cmd) {
		text = strings.ReplaceAll(text, cli.FamilyReservedSentence, reservedFamilyManualSentence)
	}
	return preserveIndentedBlocks(linkifyRelatedCommands(text, entryByPath))
}

func preserveIndentedBlocks(text string) string {
	var out []string
	var block []string
	flushBlock := func() {
		if len(block) == 0 {
			return
		}
		if len(out) > 0 && out[len(out)-1] != "" {
			out = append(out, "")
		}
		out = append(out, fencedText(strings.Join(block, "\n")), "")
		block = nil
	}
	for _, line := range strings.Split(text, "\n") {
		if unindented, ok := trimHelpIndent(line); ok {
			block = append(block, unindented)
			continue
		}
		flushBlock()
		if line == "" && len(out) > 0 && out[len(out)-1] == "" {
			continue
		}
		out = append(out, line)
	}
	flushBlock()
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func trimHelpIndent(line string) (string, bool) {
	switch {
	case strings.HasPrefix(line, "  "):
		return strings.TrimPrefix(line, "  "), true
	case strings.HasPrefix(line, "\t"):
		return strings.TrimPrefix(line, "\t"), true
	default:
		return "", false
	}
}

func subcommandLines(cmd *cobra.Command, entryByPath map[string]cli.Entry) []string {
	var lines []string
	for _, child := range cmd.Commands() {
		if !child.IsAvailableCommand() {
			continue
		}
		commandPath := strings.TrimPrefix(child.CommandPath(), "chab ")
		if commandPath == child.CommandPath() {
			continue
		}
		entry, ok := entryByPath[commandPath]
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("- [chab %s](%s) - %s", commandPath, path.Base(entry.Sidecar.DocPath), entry.Spec.Summary))
	}
	sort.Strings(lines)
	return lines
}

func hasAvailableSubcommands(cmd *cobra.Command) bool {
	for _, child := range cmd.Commands() {
		if child.IsAvailableCommand() {
			return true
		}
	}
	return false
}

// localFlagLines lists flags owned by this command only. Persistent root flags
// are inherited into every command help page, so the index documents them once.
func localFlagLines(cmd *cobra.Command, flags *pflag.FlagSet) []string {
	var lines []string
	rootFlags := cmd.Root().PersistentFlags()
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Name == "help" || rootFlags.Lookup(flag.Name) != nil {
			return
		}
		lines = append(lines, fmt.Sprintf("- `--%s` - %s", flag.Name, flag.Usage))
	})
	return lines
}

// outputModesText merges executable output modes with documentation-only
// surfaces. A command family can have no direct output mode while still being
// functional, so reserved status, not an empty mode list, controls planned
// command wording.
func outputModesText(entry cli.Entry) string {
	// Manual pages show both executable output modes and documentation-only
	// categories such as help or shell scripts. rungrad specs contain only the
	// executable modes, so the doc-only values come from the sidecar.
	modes := make([]cli.OutputMode, 0, len(entry.Spec.OutputModes)+len(entry.Sidecar.DocOnlyOutputModes))
	for _, mode := range entry.Spec.OutputModes {
		modes = append(modes, cli.OutputMode(mode))
	}
	modes = append(modes, entry.Sidecar.DocOnlyOutputModes...)
	if len(modes) == 0 {
		if entry.Sidecar.Status == cli.StatusReserved {
			return "none (planned command)"
		}
		return "none (command family)"
	}
	labels := map[cli.OutputMode]string{
		cli.OutputHuman:       "human",
		cli.OutputJSON:        "JSON",
		cli.OutputPlain:       "plain",
		cli.OutputJQ:          "jq",
		cli.OutputTemplate:    "template",
		cli.OutputHelp:        "help",
		cli.OutputShellScript: "shell script",
		cli.OutputMCPStdio:    "MCP stdio",
	}
	parts := make([]string, 0, len(modes))
	for _, mode := range modes {
		if label, ok := labels[mode]; ok {
			parts = append(parts, label)
		} else {
			parts = append(parts, string(mode))
		}
	}
	return strings.Join(parts, ", ")
}

func authenticationText(entry cli.Entry) string {
	text := "Runs without requiring a credential."
	if entry.Spec.RequiresAuth {
		text = "Requires a stored credential or `CHAB_API_KEY`."
	}
	if entry.Sidecar.AuthNote != "" {
		text += "\n\n" + entry.Sidecar.AuthNote
	}
	return text
}

// completionShells documents Cobra's generated shell children on the completion
// parent page instead of creating separate manual pages for framework-owned
// child commands.
func completionShells(cmd *cobra.Command) []string {
	var shells []string
	for _, child := range cmd.Commands() {
		if child.IsAvailableCommand() {
			shells = append(shells, "- "+child.Name())
		}
	}
	sort.Strings(shells)
	return shells
}
