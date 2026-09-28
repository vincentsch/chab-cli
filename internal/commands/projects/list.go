package projects

import (
	"fmt"
	"io"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/readservice"
)

type listFlags struct {
	Page cmdutil.CursorPaginationFlags
}

// NewListCommand builds chab project list.
func NewListCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &listFlags{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		Long: `List projects visible to the active team API key.

Pagination: --limit caps the total items fetched, --cursor starts at an opaque
API cursor, --page-size sets the per-request page size, and --all follows every
cursor page. With no list-size flag, the server's first-page default is used.

JSON output is an array of project objects with fields id, name, description,
url, status, timezone, language, limit, automate, created_at, updated_at.
Default JSON omits request and pagination metadata.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged.

Output modes: default human table, --plain tab-separated rows, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
  chab project show
  chab credits`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd, f, flags)
		},
	}
	cmd.Example = `  chab project list
  chab project list --limit 50
  chab project list --cursor <cursor> --page-size 50
  chab project list --json
  chab project list --all --json --include-meta
  chab project list --jq '.[].id'
  chab project list --template '{{range .}}{{.id}}{{"\n"}}{{end}}'`
	cmdutil.RegisterCursorPaginationFlags(cmd, &flags.Page)
	return cmd
}

// runList resolves the active runtime, fetches one or more API pages according
// to the shared pagination plan, and renders the stable project list value.
func runList(cmd *cobra.Command, f *cmdutil.Factory, flags *listFlags) error {
	opts := projectListOptions(cmd, flags)
	baseQuery, err := readservice.ProjectListQuery(opts)
	if err != nil {
		return projectUsageError(err)
	}

	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return err
	}
	plan, err := readservice.ProjectListPlan(rt, opts)
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

	outcome, err := readservice.FetchProjectList(cmd.Context(), client, plan, baseQuery)
	if err != nil {
		return output.WithCredentialContext(err, cred.Profile, cred.DisplayID)
	}
	rows := redactProjectRows(f, outcome.Rows)

	hint := cmdutil.CursorPaginationHint(plan, len(rows), outcome.Meta.CursorPagination)
	table := projectTable(rows)
	return f.WriteResultWithMeta(cmd, rows, outcome.Meta, false, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			table.Render(w)
			if hint != "" {
				fmt.Fprintln(w, hint)
			}
		},
		Plain: func(data, prose io.Writer) {
			table.RenderPlain(data, prose)
			// Plain stdout stays tabular data only; the pagination hint is prose.
			if hint != "" {
				fmt.Fprintln(prose, hint)
			}
		},
	})
}

func validProjectStatus(status string) bool {
	return readservice.ValidProjectStatus(status)
}

func cloneValues(values url.Values) url.Values {
	return readservice.CloneValues(values)
}

func projectListOptions(cmd *cobra.Command, flags *listFlags) readservice.ProjectListOptions {
	return readservice.ProjectListOptions{
		Page: readservice.CursorPaginationOptions{
			Limit:    readservice.OptionalInt{Value: flags.Page.Limit, Set: cmd.Flags().Changed("limit")},
			All:      readservice.OptionalBool{Value: flags.Page.All, Set: cmd.Flags().Changed("all")},
			Cursor:   readservice.OptionalString{Value: flags.Page.Cursor, Set: cmd.Flags().Changed("cursor")},
			PageSize: readservice.OptionalInt{Value: flags.Page.PageSize, Set: cmd.Flags().Changed("page-size")},
		},
	}
}

func projectUsageError(err error) error {
	if local, ok := err.(*readservice.ProjectUsageError); ok {
		return &usageError{detail: local.Detail}
	}
	return err
}
