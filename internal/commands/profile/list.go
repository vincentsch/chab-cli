package profile

import (
	"io"
	"sort"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

// NewListCommand builds chab profile list.
func NewListCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List configured profiles",
		Long:    profileListLong,
		Args:    cobra.NoArgs,
		Example: profileListExample,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProfileList(cmd, f)
		},
	}
}

// runProfileList merges saved profiles with the effective selection. This
// keeps one-off flag or environment selections visible without treating
// unrelated auth-only records as configured profiles.
func runProfileList(cmd *cobra.Command, f *cmdutil.Factory) error {
	file, authFile, paths, err := loadConfigAndAuth(cmd, f)
	if err != nil {
		return err
	}
	current, err := f.SelectedProfile(cmd, file)
	if err != nil {
		return err
	}

	// Include the effective selected profile even when it is not persisted, so
	// --profile/CHAB_PROFILE overrides are visible in the table.
	nameSet := map[string]bool{current: true}
	for name := range file.Profiles {
		nameSet[name] = true
	}
	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)

	rows := make([]profileJSON, 0, len(names))
	for _, name := range names {
		row, err := buildProfileJSON(file, authFile, name, current)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	result := profileListJSON{
		CurrentProfile: current,
		ConfigPath:     paths.ConfigPath,
		AuthPath:       paths.AuthPath,
		Profiles:       rows,
	}
	return f.WriteResult(cmd, result, cmdutil.HumanOutput{
		Render: func(w io.Writer) {
			renderProfileListHuman(w, result)
		},
		Plain: func(data, prose io.Writer) {
			renderProfileListPlain(data, prose, result)
		},
	})
}

const profileListLong = `List configured connection profiles and show which profile is selected for this invocation.

This command reads local config.yml and auth.json only. Stored API keys are
reported as non-secret presence, display id, and key name; the API key value is
never printed and no API request is made. This stored-auth view is separate from
the effective request credential: CHAB_API_KEY wins for API commands. Run
chab auth status to inspect the effective credential source.

An auth-only legacy installation with no config.yml retains the historical
localhost URL default; inspection shows the same destination as API commands.

JSON output is an object with current_profile, config_path, auth_path, and a
profiles array. Each profile includes name, selected, persisted, base_url,
api_base_url, locale, default_output, project_list_limit, and stored_auth.
stored_auth includes present, display_id, key_name, team_display_id, team_name,
and last_validated_at.

Output modes: default human table, --plain tab-separated rows, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
  chab profile show
  chab config list`

const profileListExample = `  chab profile list
  chab profile list --plain
  chab profile list --jq '.profiles[].name'
  chab profile list --template '{{.current_profile}}'`
