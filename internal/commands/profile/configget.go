package profile

import (
	"io"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
)

// NewConfigGetCommand builds chab config get <key>.
func NewConfigGetCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "get <key>",
		Short:   "Print one configuration value",
		Long:    configGetLong,
		Args:    cobra.ExactArgs(1),
		Example: configGetExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigGet(cmd, f, args[0])
		},
	}
}

func runConfigGet(cmd *cobra.Command, f *cmdutil.Factory, key string) error {
	file, _, err := loadConfig(cmd, f)
	if err != nil {
		return err
	}
	value, err := config.GetValue(file, key)
	if err != nil {
		return err
	}
	result := toConfigValueJSON(value)
	detail := configValueDetail(result)
	return f.WriteResult(cmd, result, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			detail.Render(w)
		},
		Plain: detail.RenderPlain,
	})
}

const configGetLong = `Print one known non-secret CLI configuration value.

Supported keys are current_profile and profiles.<name>.base_url,
profiles.<name>.api_base_url, profiles.<name>.locale,
profiles.<name>.default_output, and
profiles.<name>.defaults.project_list_limit. Profile names may contain dots.
Stored credential keys and unknown YAML keys are never shown.

All persisted profile names and known profile values are validated before any
result is printed. An invalid persisted profile therefore makes the lookup fail
without partial output, even when a different profile key was requested.

JSON output is an object with key, value, and source, where source is file,
derived, or default.

Output modes: default human detail, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
  chab config list
  chab config set`

const configGetExample = `  chab config get current_profile
  chab config get profiles.staging.base_url
  chab config get profiles.production.eu.api_base_url --jq .value
  chab config get current_profile --template '{{.value}}'`
