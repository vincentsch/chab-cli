package credits

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/readservice"
)

// NewBalanceCommand builds chab credits balance.
func NewBalanceCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "balance",
		Short: "Show the available credit balance",
		Long: `Show the available credit balance.

For a team key, this authenticated read is team-level. For a guest trial key,
spendable_balance is zero and free_credits.available is the promotional balance
that funds supported guest operations.

JSON output includes integer spendable_balance and debt, optional free_credits,
and expires as the API timestamp string or null.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged.

Output modes: default human detail, --plain labeled tab-separated fields,
--json stable JSON, and --jq/--template transforms over the documented JSON
value.

Related commands:
  chab credits transactions
  chab health`,
		Args: cobra.NoArgs,
		Example: `  chab credits balance
  chab credits balance --json
  chab credits balance --json --include-meta
  chab credits balance --jq .balance
  chab credits balance --template '{{.balance}}'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBalance(cmd, f)
		},
	}
}

// runBalance resolves local CLI state, reads the team balance, and passes the
// typed response through the shared output dispatcher.
func runBalance(cmd *cobra.Command, f *cmdutil.Factory) error {
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return err
	}
	cred, findings, err := f.Credential(rt)
	cmdutil.WarnPermissionFindings(cmd.ErrOrStderr(), findings)
	if err != nil {
		return fmt.Errorf("%w; run \"chab login\" or set CHAB_API_KEY", err)
	}
	client, err := f.APIClient(rt, cred, cmd)
	if err != nil {
		return err
	}

	data, meta, err := readservice.GetBalance(cmd.Context(), client)
	if err != nil {
		return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}

	detail := output.Detail{Nodes: balanceDetailNodes(data)}
	return f.WriteResultWithMeta(cmd, data, meta, false, cmdutil.HumanOutput{
		Render: func(w io.Writer) { detail.Render(w) },
		Plain:  detail.RenderPlain,
	})
}
