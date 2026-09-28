package system

import (
	"io"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
)

type ErrorCatalog struct {
	Errors []ErrorCatalogEntry `json:"errors"`
}

type ErrorCatalogEntry struct {
	Code       string `json:"code"`
	HTTPStatus int    `json:"http_status"`
	Retryable  bool   `json:"retryable"`
	Chargeable bool   `json:"chargeable"`
	SafeToShow bool   `json:"safe_to_show"`
	Message    string `json:"message"`
	UserAction string `json:"user_action"`
	DocsURL    string `json:"docs_url"`
	EmittedBy  string `json:"emitted_by"`
}

func NewErrorsCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "errors",
		Short: "List public API error codes",
		Long: `List public API error codes.

This command calls unauthenticated GET /v1/errors and never reads credentials.

Related commands:
  chab health
  chab doctor
  chab api get /errors`,
		Example: `  chab errors
  chab errors --plain
  chab errors --json
  chab errors --jq '.errors[] | select(.retryable)'
  chab errors --template '{{range .errors}}{{.code}}{{"\n"}}{{end}}'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runErrors(cmd, f)
		},
	}
}

func runErrors(cmd *cobra.Command, f *cmdutil.Factory) error {
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return err
	}
	client, err := f.PublicAPIClient(rt, cmd)
	if err != nil {
		return err
	}
	var data ErrorCatalog
	meta, err := client.Get(cmd.Context(), "errors", nil, &data)
	if err != nil {
		return err
	}
	table := errorCatalogTable(data.Errors)
	return f.WriteResultWithMeta(cmd, data, meta, false, cmdutil.HumanOutput{
		Render: func(w io.Writer) { table.Render(w) },
		Plain:  table.RenderPlain,
	})
}

func errorCatalogTable(entries []ErrorCatalogEntry) output.Table {
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, []string{
			entry.Code,
			strconv.Itoa(entry.HTTPStatus),
			strconv.FormatBool(entry.Retryable),
			entry.EmittedBy,
			entry.DocsURL,
		})
	}
	return output.Table{
		Columns: []string{"CODE", "HTTP", "RETRYABLE", "EMITTED_BY", "DOCS"},
		Rows:    rows,
		Empty:   "No error catalog entries returned.",
	}
}
