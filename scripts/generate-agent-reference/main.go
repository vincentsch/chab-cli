// Regenerate the installed reference from the same catalog as the native CLI.
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/vincentsch/chab-cli/internal/cli"
)

func main() {
	var b strings.Builder
	b.WriteString("# Chab Command Reference\n\nThis section is generated from the active command catalog.\n\n| Command | Summary | Output |\n| --- | --- | --- |\n")
	labels := map[cli.OutputMode]string{
		cli.OutputHuman: "human", cli.OutputJSON: "JSON", cli.OutputPlain: "plain",
		cli.OutputJQ: "jq", cli.OutputTemplate: "template", cli.OutputHelp: "help",
		cli.OutputShellScript: "shell script", cli.OutputMCPStdio: "MCP stdio",
	}
	for _, entry := range cli.Catalog() {
		var modes []string
		for _, mode := range entry.Spec.OutputModes {
			label, ok := labels[cli.OutputMode(mode)]
			if !ok {
				label = mode
			}
			modes = append(modes, label)
		}
		for _, mode := range entry.Sidecar.DocOnlyOutputModes {
			label, ok := labels[mode]
			if !ok {
				label = string(mode)
			}
			modes = append(modes, label)
		}
		output := strings.Join(modes, ", ")
		if len(modes) == 0 {
			output = "none (command family)"
			if entry.Sidecar.Status == cli.StatusReserved {
				output = "none (planned command)"
			}
		}
		fmt.Fprintf(&b, "| `chab %s` | %s | %s |\n", entry.Spec.Path, entry.Spec.Summary, output)
	}
	write(".chab-agent-skill/commands.md", []byte(b.String()))
	var manifest strings.Builder
	for _, path := range []string{".agents/skills/chab/SKILL.md", ".claude/skills/chab/SKILL.md", ".chab-agent-skill/README.md", ".chab-agent-skill/commands.md"} {
		data, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(&manifest, "%x  %s\n", sha256.Sum256(data), path)
	}
	write(".chab-agent-skill/manifest.sha256", []byte(manifest.String()))
}

func write(path string, data []byte) {
	if err := os.WriteFile(path, data, 0644); err != nil {
		panic(err)
	}
}
