package rawapi

import (
	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

// NewAPICommand builds the raw API family parent.
func NewAPICommand(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api",
		Short: "Call public API endpoints directly",
		Long:  "Call public API endpoints directly when a first-class command is not available. Paths are confined below the resolved API base, and command leaves reuse the active profile, credential, locale, retry, error, and output behavior. Raw POST refuses documented v1 routes that return one-time plaintext secrets; use the dedicated private-output workflow for those operations.\n\nRelated commands:\n  chab login\n  chab health",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunFamilyHelpOrUnknown(cmd, args)
		},
	}
	cmd.Example = "  chab api get /me\n  chab api get /credits"
	cmd.AddCommand(
		NewGetCommand(f),
		NewPostCommand(f),
		NewPatchCommand(f),
		NewDeleteCommand(f),
	)
	return cmd
}
