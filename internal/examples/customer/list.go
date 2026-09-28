package customer

import (
	"fmt"
	"io"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/rungrad"
)

// defaultListLimit keeps the example self-contained. Production list commands
// usually resolve their default limit from config after runtime resolution.
const defaultListLimit = 30

// listFlags groups the parsed local list controls for one command invocation.
type listFlags struct {
	Page cmdutil.PaginationFlags
}

// NewListCommand builds chab customer list for test-owned command trees.
func NewListCommand(f *cmdutil.Factory) *rungrad.Command {
	flags := &listFlags{}
	return &rungrad.Command{
		Use:   "list",
		Short: "List example customers",
		Long: `List example customers through a teaching artifact for the SaaS CLI
authoring guide, compiled only into test command trees.

Pagination: --limit caps the total items fetched across pages, --all fetches
every page, and --page/--per-page fetch one exact page. With no list-size flag
the default limit is 30.

JSON output is an array of customer objects with fields id, name, status,
created_at. Default JSON omits request and pagination metadata.

Add --include-meta to JSON, --jq, or --template output to wrap this command's
value under data and expose request id, pagination, rate-limit, and retry
metadata under meta.

Output modes: default human table, --plain tab-separated rows, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
  chab customer create`,
		Args: cobra.NoArgs,
		Configure: func(cmd *cobra.Command) {
			cmd.Example = `  chab customer list
  chab customer list --all
  chab customer list --include-meta --json`
			cmdutil.RegisterPaginationFlags(cmd, &flags.Page)
		},
		Run: func(_ *rungrad.Factory, cmd *cobra.Command, _ []string) error {
			return runList(cmd, f, flags)
		},
	}
}

// runList validates local pagination flags before resolving runtime state, then
// fetches one or more pages and renders the module-owned customer list shape.
func runList(cmd *cobra.Command, f *cmdutil.Factory, flags *listFlags) error {
	plan, err := cmdutil.ResolveListPlan(cmd, &flags.Page, cmdutil.DefaultResolvedLimit, defaultListLimit)
	if err != nil {
		return err
	}
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

	outcome, err := cmdutil.FetchPages(plan, func(page, perPage int) ([]customerJSON, api.ResponseMeta, error) {
		// Build a fresh query map for each callback invocation so page controls
		// cannot leak between requests as the pagination helper loops.
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		if perPage > 0 {
			q.Set("per_page", strconv.Itoa(perPage))
		}
		var rows []customerJSON
		meta, getErr := client.Get(cmd.Context(), api.Path("customers"), q, &rows)
		if getErr != nil {
			return nil, api.ResponseMeta{}, getErr
		}
		return rows, meta, nil
	})
	if err != nil {
		return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}

	hint := cmdutil.PaginationHint(outcome.Mode, len(outcome.Rows), outcome.Page, outcome.Meta.Pagination)
	table := customerTable(outcome.Rows)
	return f.WriteResultWithMeta(cmd, outcome.Rows, outcome.Meta, false, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			table.Render(w)
			if hint != "" {
				fmt.Fprintln(w, hint)
			}
		},
		Plain: func(data, prose io.Writer) {
			table.RenderPlain(data, prose)
			if hint != "" {
				fmt.Fprintln(prose, hint)
			}
		},
	})
}
