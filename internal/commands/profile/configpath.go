package profile

import (
	"io"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

// NewConfigPathCommand builds chab config path.
func NewConfigPathCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "path",
		Short:   "Print the config and auth file paths",
		Long:    configPathLong,
		Args:    cobra.NoArgs,
		Example: configPathExample,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigPath(cmd, f)
		},
	}
}

func runConfigPath(cmd *cobra.Command, f *cmdutil.Factory) error {
	paths, err := f.ResolveLocalPaths(cmd)
	if err != nil {
		return err
	}
	result := configPathJSON{ConfigPath: paths.ConfigPath, AuthPath: paths.AuthPath}
	detail := configPathDetail(result)
	return f.WriteResult(cmd, result, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			detail.Render(w)
		},
		Plain: detail.RenderPlain,
	})
}

const configPathLong = `Print the local config and auth file paths selected for this invocation.

This command resolves --config, --auth-file, CHAB_CONFIG, CHAB_AUTH_FILE, and the
built-in defaults without loading or parsing either file. It still works when
config.yml is malformed.

JSON output is an object with config_path and auth_path.

Output modes: default human detail, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
  chab config list
  chab profile list`

const configPathExample = `  chab config path
  chab config path --plain
  chab config path --jq .config_path
  chab config path --template '{{.config_path}}'`
