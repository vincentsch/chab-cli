package profile

import (
	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

// NewProfileCommand builds the profile family parent.
func NewProfileCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "profile",
		Short:   "Manage connection profiles",
		Long:    "Manage local connection profiles for product environments without contacting the API. Profiles hold base URL and locale defaults while credentials are stored separately by chab login. Inspection validates every persisted profile before printing output.\n\nRelated commands:\n  chab login\n  chab auth status\n  chab config",
		Args:    cobra.ArbitraryArgs,
		Example: "  chab profile list\n  chab profile show staging",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunFamilyHelpOrUnknown(cmd, args)
		},
	}
	cmd.AddCommand(
		NewListCommand(f),
		NewShowCommand(f),
		NewCreateCommand(f),
		NewUseCommand(f),
		NewDeleteCommand(f),
	)
	return cmd
}

// NewConfigCommand builds the config family parent.
func NewConfigCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Inspect and edit non-secret CLI configuration",
		Long:    "Inspect and edit local non-secret CLI configuration such as profile defaults and product URL settings without contacting the API. Config files must contain one YAML document with unique mapping keys. Inspection validates every persisted profile before printing output. Stored API keys are never shown or set here.\n\nRelated commands:\n  chab profile\n  chab auth status",
		Args:    cobra.ArbitraryArgs,
		Example: "  chab config list\n  chab config get current_profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunFamilyHelpOrUnknown(cmd, args)
		},
	}
	cmd.AddCommand(
		NewConfigPathCommand(f),
		NewConfigListCommand(f),
		NewConfigGetCommand(f),
		NewConfigSetCommand(f),
	)
	return cmd
}
