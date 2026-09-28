package profile

import (
	"io"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
)

// NewShowCommand builds chab profile show [name].
func NewShowCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "show [name]",
		Short:   "Show a profile's settings",
		Long:    profileShowLong,
		Args:    cobra.MaximumNArgs(1),
		Example: profileShowExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileShow(cmd, f, args)
		},
	}
}

// runProfileShow displays either the explicit name or the effective selection.
// Valid unsaved names use built-in settings and are marked as not persisted.
func runProfileShow(cmd *cobra.Command, f *cmdutil.Factory, args []string) error {
	file, authFile, _, err := loadConfigAndAuth(cmd, f)
	if err != nil {
		return err
	}
	current, err := f.SelectedProfile(cmd, file)
	if err != nil {
		return err
	}
	name := current
	if len(args) > 0 {
		name = args[0]
		if err := config.ValidateProfileName(name); err != nil {
			return err
		}
	}
	if err := config.ValidateProfiles(file); err != nil {
		return err
	}

	result, err := buildProfileJSON(file, authFile, name, current)
	if err != nil {
		return err
	}
	detail := profileDetail(result)
	return f.WriteResult(cmd, result, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			detail.Render(w)
		},
		Plain: detail.RenderPlain,
	})
}

const profileShowLong = `Show one profile's local, non-secret settings.

When no name is supplied, the selected profile is resolved from --profile,
CHAB_PROFILE, current_profile, or the built-in local default. Missing profiles
show built-in defaults and are marked as not saved. Stored API keys are reported
only as non-secret presence and display metadata; the API key value is never
printed and no API request is made. This stored-auth view is separate from the
effective request credential: CHAB_API_KEY wins for API commands. Run
chab auth status to inspect the effective credential source.

An auth-only legacy installation with no config.yml retains the historical
localhost URL default; inspection shows the same destination as API commands.

All persisted profile names and known profile values are validated before any
result is printed. An invalid persisted profile therefore makes inspection fail
without partial output, even when a different profile was requested.

JSON output is one profile object with name, selected, persisted, base_url,
api_base_url, locale, default_output, project_list_limit, and stored_auth.
stored_auth includes present, display_id, key_name, team_display_id, team_name,
and last_validated_at.

Output modes: default human detail, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
  chab profile list
  chab config get`

const profileShowExample = `  chab profile show
  chab profile show staging
  chab profile show staging --jq .api_base_url
  chab profile show staging --template '{{.name}}'`
