package management

import (
	"net/url"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

type usageFlags struct {
	Start    string
	End      string
	Timezone string
	Bucket   string
	GroupBy  string
}

// NewUsageCommand builds chab usage.
func NewUsageCommand(f *cmdutil.Factory) *cobra.Command {
	flags := &usageFlags{}
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Show team API usage",
		Long: `Show team API usage visible to the active API key.

The server owns project access, token visibility, date-window limits and plan
policy. Optional filters map directly to the public API query: --start, --end,
--timezone, --bucket (day or hour), and --group-by (operation_key, family,
project, or token). Browser device login currently grants only read-oriented
management scopes; use a manually created team API key when the server rejects
broader scopes with invalid_scope or api_scope_missing.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
  chab credits balance
  chab billing show`,
		Args: cobra.NoArgs,
		Example: `  chab usage
  chab usage --start START_TIME --end END_TIME
  chab usage --bucket hour --group-by family --json
  chab usage --json --include-meta
  chab usage --jq '.groups[].final_credits'
  chab usage --template '{{.total_credits}}'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if flags.Start != "" {
				q.Set("start", flags.Start)
			}
			if flags.End != "" {
				q.Set("end", flags.End)
			}
			if flags.Timezone != "" {
				q.Set("timezone", flags.Timezone)
			}
			if flags.Bucket != "" {
				q.Set("bucket", flags.Bucket)
			}
			if flags.GroupBy != "" {
				q.Set("group_by", flags.GroupBy)
			}
			return readOnlyRaw(cmd, f, "usage", q)
		},
	}
	cmd.Flags().StringVar(&flags.Start, "start", "", "usage range start as an API date-time")
	cmd.Flags().StringVar(&flags.End, "end", "", "usage range end as an API date-time")
	cmd.Flags().StringVar(&flags.Timezone, "timezone", "", "IANA timezone for bucket boundaries")
	cmd.Flags().StringVar(&flags.Bucket, "bucket", "", "usage bucket: day or hour")
	cmd.Flags().StringVar(&flags.GroupBy, "group-by", "", "usage grouping: operation_key, family, project, or token")
	return cmd
}
