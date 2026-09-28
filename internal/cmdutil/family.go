package cmdutil

import (
	"fmt"

	"github.com/spf13/cobra"
)

// UnknownSubcommandError is used for typos under a command family. Cobra would
// otherwise treat some non-leaf typos as a request for parent help.
type UnknownSubcommandError struct {
	Name string
	Path string
}

func (e *UnknownSubcommandError) Error() string {
	return fmt.Sprintf("unknown command %q for %q", e.Name, e.Path)
}

func (e *UnknownSubcommandError) ExitCode() int {
	return 1
}

// RunFamilyHelpOrUnknown shows family help for bare invocations and turns
// unmatched subcommand words into typed local usage errors.
func RunFamilyHelpOrUnknown(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	return &UnknownSubcommandError{Name: args[0], Path: cmd.CommandPath()}
}
