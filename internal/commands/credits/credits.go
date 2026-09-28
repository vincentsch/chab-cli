package credits

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

// NewCreditsCommand builds the functional credits family and registers its
// runnable balance and transaction-history children. Bare invocation shows
// help, and an unmatched subcommand becomes a typed usage error.
func NewCreditsCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credits",
		Short: "Inspect team credits",
		Long: strings.TrimSpace(`Inspect team credits through the product API.

Balance inspection and cursor-paginated transaction history are available as
team-level reads.

Credits are a team-level resource rather than Project-scoped. A key with
credits read capability reports the whole team's balance and transactions.

Related commands:
  chab login
  chab health`),
		Args: cobra.ArbitraryArgs,
		Example: `  chab credits balance
  chab credits transactions`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunFamilyHelpOrUnknown(cmd, args)
		},
	}
	cmd.AddCommand(
		NewBalanceCommand(f),
		NewTransactionsCommand(f),
	)
	return cmd
}
