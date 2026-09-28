// Package cmdutil contains command-runtime adapters shared by CLI wiring and
// functional command packages.
package cmdutil

import (
	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/runtimeflags"
)

// RuntimeFlagOverrides converts already-parsed persistent flags into config
// resolution inputs. It does not resolve files, read env vars, or create state.
func RuntimeFlagOverrides(cmd *cobra.Command) config.FlagOverrides {
	return runtimeflags.Overrides(cmd)
}

// DebugEnabled reports the parsed global --debug state for the invocation.
func DebugEnabled(cmd *cobra.Command) bool {
	return boolFlag(cmd, "debug")
}

// JSONEnabled reports the parsed global --json state for the invocation.
func JSONEnabled(cmd *cobra.Command) bool {
	return boolFlag(cmd, "json")
}

// IncludeMetaEnabled reports the parsed global --include-meta state.
func IncludeMetaEnabled(cmd *cobra.Command) bool {
	return boolFlag(cmd, "include-meta")
}

// NoPromptEnabled reports the parsed global --no-prompt state.
func NoPromptEnabled(cmd *cobra.Command) bool {
	return boolFlag(cmd, "no-prompt")
}

// YesEnabled reports the parsed global --yes state.
func YesEnabled(cmd *cobra.Command) bool {
	return boolFlag(cmd, "yes")
}

// PlainEnabled reports the parsed global --plain state.
func PlainEnabled(cmd *cobra.Command) bool {
	return boolFlag(cmd, "plain")
}

// NoColorEnabled reports the parsed global --no-color state.
func NoColorEnabled(cmd *cobra.Command) bool {
	return boolFlag(cmd, "no-color")
}

// NoANSIEnabled reports the parsed global --no-ansi state.
func NoANSIEnabled(cmd *cobra.Command) bool {
	return boolFlag(cmd, "no-ansi")
}

// NoPagerEnabled reports the parsed global --no-pager state.
func NoPagerEnabled(cmd *cobra.Command) bool {
	return boolFlag(cmd, "no-pager")
}

// JQExpr returns the --jq expression and whether the flag was explicitly set.
func JQExpr(cmd *cobra.Command) (string, bool) {
	return stringFlag(cmd, "jq")
}

// TemplateExpr returns the --template text and whether the flag was explicitly
// set.
func TemplateExpr(cmd *cobra.Command) (string, bool) {
	return stringFlag(cmd, "template")
}

// MachineOutputEnabled reports whether stdout is controlled by JSON or a
// transform mode. It deliberately keys jq/template off Flag.Changed so
// --jq="" remains an explicit request for identity filtering.
func MachineOutputEnabled(cmd *cobra.Command) bool {
	_, jq := JQExpr(cmd)
	_, tmpl := TemplateExpr(cmd)
	return JSONEnabled(cmd) || jq || tmpl
}

func boolFlag(cmd *cobra.Command, name string) bool {
	if cmd == nil || cmd.Root() == nil {
		return false
	}
	flag := cmd.Root().PersistentFlags().Lookup(name)
	return flag != nil && flag.Value.String() == "true"
}

func stringFlag(cmd *cobra.Command, name string) (string, bool) {
	if cmd == nil || cmd.Root() == nil {
		return "", false
	}
	flag := cmd.Root().PersistentFlags().Lookup(name)
	if flag == nil {
		return "", false
	}
	return flag.Value.String(), flag.Changed
}
