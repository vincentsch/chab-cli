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

// transactionsFlags keeps the parsed filters and pagination settings together
// for one transaction-list invocation.
type transactionsFlags struct {
	Page cmdutil.CursorPaginationFlags
}

// NewTransactionsCommand builds the active native transaction-list command.
func NewTransactionsCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &transactionsFlags{}
	cmd := &cobra.Command{
		Use:   "transactions",
		Short: "List credit transactions",
		Long: `List credit transactions for the authenticated team.

Credits are team-level: the transaction history is not narrowed by a
selected-project key scope.

Pagination uses Chab cursor metadata. --cursor starts from a server-provided
cursor, --page-size sets the API request limit, --limit caps total rows, and
--all follows next_cursor until the API reports no more pages.

JSON output is an array of transaction objects with fields id, occurred_at,
kind, amount, resulting_spendable_balance, expires_at, operation_id,
purchase_id, and description. description is localized display text and must
not be parsed for behavior. amount is a signed integer credit unit. Default
JSON omits request and pagination metadata.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged.

Output modes: default human table, --plain tab-separated rows, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
  chab credits balance
  chab health`,
		Args: cobra.NoArgs,
		Example: `  chab credits transactions
  chab credits transactions --limit 250
  chab credits transactions --cursor eyJpZCI6MX0 --page-size 25
  chab credits transactions --json
  chab credits transactions --include-meta --jq .meta.cursor.next_cursor
  chab credits transactions --jq '.[].amount'
  chab credits transactions --template '{{range .}}{{.id}} {{.amount}}{{"\n"}}{{end}}'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTransactions(cmd, f, flags)
		},
	}
	cmdutil.RegisterCursorPaginationFlags(cmd, &flags.Page)
	return cmd
}

// runTransactions validates local input, resolves credentials, fetches the
// requested pages, and renders prose context only outside machine output.
func runTransactions(cmd *cobra.Command, f *cmdutil.Factory, flags *transactionsFlags) error {
	baseQuery, err := buildTransactionQuery(cmd, flags)
	if err != nil {
		return err
	}

	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return err
	}
	// Flag validation fails here, before any credential lookup or HTTP request.
	plan, err := readservice.TransactionListPlan(transactionListOptions(cmd, flags))
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

	outcome, err := readservice.FetchTransactions(cmd.Context(), client, plan, baseQuery)
	if err != nil {
		return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}

	hint := cmdutil.CursorPaginationHint(plan, len(outcome.Rows), outcome.Meta.CursorPagination)
	table := transactionTable(outcome.Rows)
	// The normal machine value remains the transaction rows. The shared writer
	// adds response metadata only when the supported opt-in flag is active.
	return f.WriteResultWithMeta(cmd, outcome.Rows, outcome.Meta, false, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			table.Render(w)
			if hint != "" {
				fmt.Fprintln(w, hint)
			}
		},
		Plain: func(data, prose io.Writer) {
			table.RenderPlain(data, prose)
			// Plain stdout stays tabular data only; context and hints are prose.
			if hint != "" {
				fmt.Fprintln(prose, hint)
			}
		},
	})
}
