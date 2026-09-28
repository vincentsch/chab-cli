package projects

import "github.com/spf13/cobra"

// dryRunEnabled reads only the active Project mutation's local flag. Keeping
// this lookup local prevents a parent or unrelated command from implicitly
// gaining dry-run behavior.
func dryRunEnabled(cmd *cobra.Command) bool {
	if cmd == nil || cmd.LocalNonPersistentFlags().Lookup("dry-run") == nil {
		return false
	}
	enabled, err := cmd.LocalNonPersistentFlags().GetBool("dry-run")
	return err == nil && enabled
}
