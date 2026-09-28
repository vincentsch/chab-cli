// Package retainedcmdutil contains helpers needed by retained later command
// packages that are not wired into the active Cobra shell.
package retainedcmdutil

import (
	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

// IncludeMetaEnabled reports the parsed global --include-meta state.
func IncludeMetaEnabled(cmd *cobra.Command) bool {
	return boolFlag(cmd, "include-meta")
}

// DryRunEnabled reports the parsed global --dry-run state.
func DryRunEnabled(cmd *cobra.Command) bool {
	return boolFlag(cmd, "dry-run")
}

// QuietEnabled reports the parsed global --quiet state.
func QuietEnabled(cmd *cobra.Command) bool {
	return boolFlag(cmd, "quiet")
}

// PlainEnabled reports the parsed global --plain state for the invocation.
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

// MachineOutputEnabled reports whether stdout is controlled by a machine mode.
func MachineOutputEnabled(cmd *cobra.Command) bool {
	_, jq := JQExpr(cmd)
	_, tmpl := TemplateExpr(cmd)
	return cmdutil.JSONEnabled(cmd) || jq || tmpl
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
