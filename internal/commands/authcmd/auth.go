package authcmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

// NewAuthCommand builds the auth family.
func NewAuthCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Inspect and manage API authentication",
		Long: strings.TrimSpace(`Inspect and manage API authentication for chab.

Team API keys are created and revoked in the product web app. This family
stores an API key after browser authorization or manual entry validates it
with GET /v1/me, reports the effective credential and token state,
removes local stored credentials, and shows the effective non-secret runtime
values used by CI invocations.

chab never prints an API key. Only non-secret runtime, token, and team metadata
can appear in output.

Related commands:
  chab login
  chab whoami
  chab profile show
  chab doctor`),
		Example: `  chab auth login
  chab auth status
  chab auth env --json
  chab auth logout`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunFamilyHelpOrUnknown(cmd, args)
		},
	}
	cmd.AddCommand(NewLoginCommand(f), NewStatusCommand(f), NewEnvCommand(f), NewLogoutCommand(f))
	return cmd
}
