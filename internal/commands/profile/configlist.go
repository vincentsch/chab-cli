package profile

import (
	"io"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
)

// NewConfigListCommand builds chab config list.
func NewConfigListCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List configuration values",
		Long:    configListLong,
		Args:    cobra.NoArgs,
		Example: configListExample,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigList(cmd, f)
		},
	}
}

func runConfigList(cmd *cobra.Command, f *cmdutil.Factory) error {
	file, paths, err := loadConfig(cmd, f)
	if err != nil {
		return err
	}
	values, err := config.ListValues(file)
	if err != nil {
		return err
	}
	rows := make([]configValueJSON, 0, len(values))
	for _, value := range values {
		rows = append(rows, toConfigValueJSON(value))
	}
	result := configListJSON{ConfigPath: paths.ConfigPath, Values: rows}
	return f.WriteResult(cmd, result, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			renderConfigListHuman(w, result)
		},
		Plain: func(data, prose io.Writer) {
			renderConfigListPlain(data, prose, result)
		},
	})
}

const configListLong = `List known non-secret CLI configuration values.

The output includes current_profile and known profile fields such as base_url,
api_base_url, locale, default_output, and defaults.project_list_limit. Unknown
YAML keys and stored credentials are never shown.

When config.yml is absent and the built-in local profile has a stored key,
its implicit URL values remain localhost. This list describes config/default
values, not a CHAB_PROFILE override; use auth env for the selected runtime.

JSON output is an object with config_path and a values array. Each value has key,
value, and source, where source is file, derived, or default.

Output modes: default human table, --plain tab-separated rows, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
  chab config get
  chab profile show`

const configListExample = `  chab config list
  chab config list --plain
  chab config list --jq '.values[].key'
  chab config list --template '{{range .values}}{{.key}}{{"\n"}}{{end}}'`
