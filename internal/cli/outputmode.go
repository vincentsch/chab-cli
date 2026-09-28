package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/outputtransform"
)

// outputModeSupport is the catalog-derived capability snapshot used by the
// root preflight before any command callback runs.
type outputModeSupport struct {
	Plain    bool
	JQ       bool
	Template bool
	Meta     bool
	// Framework records catalog ownership for help/completion rows. The guard
	// still detects those paths by command name because Cobra can execute
	// generated completion paths that are not catalog entries.
	Framework bool
}

// installOutputModeGuard wires validation at the root so invalid output-mode
// requests fail before command callbacks can read config, auth, stdin, or HTTP.
func installOutputModeGuard(root *cobra.Command, entries []Entry) {
	supports := catalogOutputModeSupport(entries)
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		return guardOutputModes(cmd, supports)
	}
}

func catalogOutputModeSupport(entries []Entry) map[string]outputModeSupport {
	out := make(map[string]outputModeSupport, len(entries))
	for _, entry := range entries {
		support := outputModeSupport{Meta: entry.Spec.SupportsMeta}
		support.Framework = entry.Sidecar.FrameworkOwned
		for _, mode := range entry.Spec.OutputModes {
			switch mode {
			case string(OutputPlain):
				support.Plain = true
			case string(OutputJQ):
				support.JQ = true
			case string(OutputTemplate):
				support.Template = true
			}
		}
		out[entry.Spec.Path] = support
	}
	return out
}

// guardOutputModes enforces cross-flag conflicts and per-command support for
// modes that are more specific than default human or inherited JSON output.
func guardOutputModes(cmd *cobra.Command, supports map[string]outputModeSupport) error {
	jqExpr, jqSet := cmdutil.JQExpr(cmd)
	templateExpr, templateSet := cmdutil.TemplateExpr(cmd)
	plain := cmdutil.PlainEnabled(cmd)
	includeMeta := cmdutil.IncludeMetaEnabled(cmd)

	path := normalizedCommandPath(cmd)
	if path == "" {
		return nil
	}
	// Metadata has stricter path and machine-mode rules than the other output
	// flags. Check it before their early returns so JSON-only commands and named
	// help or completion paths cannot bypass metadata validation.
	if includeMeta {
		support, ok := supports[path]
		if !ok || !support.Meta {
			return fmt.Errorf("--include-meta is not supported for %q", cmd.CommandPath())
		}
		if localDryRunEnabled(cmd) {
			return fmt.Errorf("--include-meta cannot be used with --dry-run")
		}
		if !cmdutil.JSONEnabled(cmd) && !jqSet && !templateSet {
			return fmt.Errorf("--include-meta requires --json, --jq, or --template")
		}
	}
	if !jqSet && !templateSet && !plain {
		return nil
	}
	// Help and completion are Cobra-owned output surfaces. They may execute
	// through synthetic command paths, so they are kept out of catalog support
	// lookup and never routed through shared output transforms.
	if isCobraOwnedOutputPath(path) {
		if jqSet || templateSet {
			return fmt.Errorf("--jq and --template are not supported for %q", cmd.CommandPath())
		}
		return nil
	}

	if jqSet && templateSet {
		return fmt.Errorf("--jq and --template cannot be used together")
	}
	if plain && (cmdutil.JSONEnabled(cmd) || jqSet || templateSet) {
		return fmt.Errorf("--plain cannot be used with --json, --jq, or --template")
	}

	support, ok := supports[path]
	if !ok {
		return fmt.Errorf("output modes are not supported for %q", cmd.CommandPath())
	}
	if jqSet {
		if !support.JQ {
			return fmt.Errorf("--jq is not supported for %q", cmd.CommandPath())
		}
		return outputtransform.ValidateJQ(jqExpr)
	}
	if templateSet {
		if !support.Template {
			return fmt.Errorf("--template is not supported for %q", cmd.CommandPath())
		}
		return outputtransform.ValidateTemplate(templateExpr)
	}
	if plain && !support.Plain {
		return fmt.Errorf("--plain is not supported for %q", cmd.CommandPath())
	}
	return nil
}

// localDryRunEnabled reads only the leaf's parsed --dry-run value. Missing,
// invalid, and explicitly false values are inactive here; Cobra retains
// responsibility for normal flag-parse errors.
func localDryRunEnabled(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.LocalNonPersistentFlags().Lookup("dry-run")
	if flag == nil {
		return false
	}
	value, err := strconv.ParseBool(flag.Value.String())
	return err == nil && value
}

func normalizedCommandPath(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	path := strings.TrimSpace(cmd.CommandPath())
	if path == "" {
		return ""
	}
	rootName := ""
	if root := cmd.Root(); root != nil {
		rootName = root.Name()
	}
	if rootName == "" {
		rootName = "chab"
	}
	path = strings.TrimSpace(strings.TrimPrefix(path, rootName))
	return strings.Join(strings.Fields(path), " ")
}

func isCobraOwnedOutputPath(path string) bool {
	return path == "help" ||
		strings.HasPrefix(path, "help ") ||
		path == "completion" ||
		strings.HasPrefix(path, "completion ") ||
		path == "__complete" ||
		strings.HasPrefix(path, "__complete ")
}
