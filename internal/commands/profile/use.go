package profile

import (
	"io"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
)

// NewUseCommand builds chab profile use <name>.
func NewUseCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "use <name>",
		Short:   "Switch the current profile",
		Long:    profileUseLong,
		Args:    cobra.ExactArgs(1),
		Example: profileUseExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileUse(cmd, f, args[0])
		},
	}
}

func runProfileUse(cmd *cobra.Command, f *cmdutil.Factory, name string) error {
	if err := config.ValidateProfileName(name); err != nil {
		return err
	}
	file, paths, err := loadConfig(cmd, f)
	if err != nil {
		return err
	}
	if _, ok := file.Profiles[name]; !ok {
		return &usageError{detail: "profile " + strconv.Quote(name) + " does not exist; create it with chab profile create"}
	}
	if err := config.SetCurrentProfile(file, name); err != nil {
		return err
	}
	if err := config.Write(paths.ConfigPath, file); err != nil {
		return err
	}
	summary := output.MutationSummary{Action: "Switched to", Resource: "profile", Name: name}
	return f.WriteResult(cmd, nil, cmdutil.HumanOutput{
		Render: mutationHuman(summary),
		Plain:  func(data, prose io.Writer) { mutationPlain(summary)(data, prose) },
	})
}

const profileUseLong = `Switch the current profile stored in local config.yml.

The target profile must already exist in config.yml. This command may inspect
which profiles have a stored key to preserve legacy URL defaults; it never
displays the key, validates API access, or contacts the API.

This command has no stable JSON shape, so --jq and --template are not supported.

Related commands:
  chab profile list
  chab login`

const profileUseExample = `  chab profile use staging
  chab profile use staging --plain`
