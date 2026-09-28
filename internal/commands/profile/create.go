package profile

import (
	"io"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
)

// NewCreateCommand builds chab profile create <name>.
func NewCreateCommand(f *cmdutil.Factory) *cobra.Command {
	var projectListLimit int
	cmd := &cobra.Command{
		Use:     "create <name>",
		Short:   "Create a profile",
		Long:    profileCreateLong,
		Args:    cobra.ExactArgs(1),
		Example: profileCreateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileCreate(cmd, f, args[0], projectListLimit)
		},
	}
	cmd.Flags().IntVar(&projectListLimit, "project-list-limit", 0, "default project list limit for the profile")
	return cmd
}

// runProfileCreate validates command input before reading local state, then
// claims the profile name only when neither config nor auth already owns it.
// Successful creation writes non-secret config and leaves auth untouched.
func runProfileCreate(cmd *cobra.Command, f *cmdutil.Factory, name string, projectListLimit int) error {
	if err := config.ValidateProfileName(name); err != nil {
		return err
	}
	flags := cmdutil.RuntimeFlagOverrides(cmd)
	if !flags.BaseURL.Set {
		return &usageError{detail: "profile create requires --base-url"}
	}
	if flags.BaseURL.Value == "" {
		return &usageError{detail: "--base-url must not be empty"}
	}
	if flags.APIBaseURL.Set && flags.APIBaseURL.Value == "" {
		return &usageError{detail: "--api-base-url must not be empty"}
	}
	if flags.Locale.Set && flags.Locale.Value == "" {
		return &usageError{detail: "--locale must not be empty"}
	}
	limitSet := cmd.Flags().Changed("project-list-limit")
	if limitSet && projectListLimit <= 0 {
		return &usageError{detail: "--project-list-limit must be a positive integer"}
	}

	profile := config.Profile{BaseURL: flags.BaseURL.Value}
	if flags.APIBaseURL.Set {
		profile.APIBaseURL = flags.APIBaseURL.Value
	}
	if flags.Locale.Set {
		profile.Locale = flags.Locale.Value
	}
	if limitSet {
		profile.Defaults.ProjectListLimit = projectListLimit
	}
	// Validate every explicit profile value before local files are inspected.
	validationFile := &config.File{Profiles: map[string]config.Profile{}}
	if err := config.UpsertProfile(validationFile, name, profile); err != nil {
		return err
	}

	file, authFile, paths, err := loadConfigAndAuth(cmd, f)
	if err != nil {
		return err
	}
	if _, exists := file.Profiles[name]; exists {
		return &usageError{detail: "profile " + strconv.Quote(name) + " already exists; change it with chab config set or delete it first"}
	}
	if _, exists := authFile.Profiles[name]; exists {
		return &usageError{detail: "profile " + strconv.Quote(name) + " already has a stored credential; remove it first with chab logout --profile " + name}
	}

	// When the first saved profile is not "local", leaving current_profile to
	// default to local would create an immediately dangling selection.
	firstProfile := len(file.Profiles) == 0
	if err := config.UpsertProfile(file, name, profile); err != nil {
		return err
	}
	if firstProfile {
		if err := config.SetCurrentProfile(file, name); err != nil {
			return err
		}
	}
	if err := config.Write(paths.ConfigPath, file); err != nil {
		return err
	}

	view, _, err := config.ResolveProfileView(file, name)
	if err != nil {
		return err
	}
	notes := []output.Node{
		output.Guidance("Stored credentials are managed separately; run chab login --profile " + name + " to add an API key."),
	}
	if firstProfile {
		notes = append([]output.Node{output.Guidance("Set as the current profile.")}, notes...)
	}
	summary := output.MutationSummary{
		Action:   "Created",
		Resource: "profile",
		Name:     name,
		Fields: []output.Node{
			output.Field("base_url", view.BaseURL),
			output.Field("api_base_url", view.APIBaseURL),
			output.Field("locale", localeDetailCell(view.Locale)),
			output.Field("project_list_limit", strconv.Itoa(view.ProjectListLimit)),
		},
		Notes: notes,
	}
	return f.WriteResult(cmd, nil, cmdutil.HumanOutput{
		Render: mutationHuman(summary),
		Plain:  func(data, prose io.Writer) { mutationPlain(summary)(data, prose) },
	})
}

const profileCreateLong = `Create a saved profile in local config.yml.

Pass the global --base-url flag to set the product base URL for the new profile.
Optional global --api-base-url and --locale flags set that profile's API base URL
and locale. The API base URL is derived from --base-url when --api-base-url is
omitted. Use --project-list-limit to set the profile's default project list
limit. Environment variables such as CHAB_BASE_URL are not used as create input.

This command writes only non-secret config. Stored credentials are managed
separately with chab login and are never created or printed here.

This command has no stable JSON shape, so --jq and --template are not supported.

Related commands:
  chab profile use
  chab login`

const profileCreateExample = `  chab profile create staging --base-url https://staging.example.test
  chab profile create staging --base-url https://staging.example.test --locale en --project-list-limit 50
  chab profile create staging --base-url https://staging.example.test --plain`
