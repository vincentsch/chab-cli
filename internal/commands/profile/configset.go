package profile

import (
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
)

// NewConfigSetCommand builds chab config set <key> <value>.
func NewConfigSetCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "set <key> <value>",
		Short:   "Set one configuration value",
		Long:    configSetLong,
		Args:    cobra.ExactArgs(2),
		Example: configSetExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSet(cmd, f, args[0], args[1])
		},
	}
}

// runConfigSet changes one known non-secret value in memory, applies any
// credential-safety checks, and writes config only after every check succeeds.
func runConfigSet(cmd *cobra.Command, f *cmdutil.Factory, key, value string) error {
	file, paths, err := loadConfig(cmd, f)
	if err != nil {
		return err
	}

	var rederivedAPIBaseURL string
	if key == "current_profile" {
		if _, ok := file.Profiles[value]; !ok {
			return &usageError{detail: "profile " + strconv.Quote(value) + " does not exist; create it with chab profile create"}
		}
		if err := config.SetCurrentProfile(file, value); err != nil {
			return err
		}
	} else {
		name, suffix, ok := config.ParseProfileKey(key)
		if !ok {
			// Only a small known-key surface is writable. Secret-looking keys are
			// called out separately so users go through login/logout instead of
			// trying to place credentials in config.yml.
			if secretLikeConfigKey(key) {
				return &usageError{detail: "refusing to set a secret-bearing key through config; manage credentials with chab login and chab logout"}
			}
			return &usageError{detail: "unsupported config key " + strconv.Quote(key)}
		}
		if _, exists := file.Profiles[name]; !exists {
			return &usageError{detail: "profile " + strconv.Quote(name) + " does not exist; create it with chab profile create"}
		}
		switch suffix {
		case "base_url":
			// Stored credentials are associated with the effective API URL, not
			// the literal base_url spelling. Compare resolved destinations so
			// normalized no-ops and unchanged custom API URLs remain allowed.
			authFile, authErr := loadAuth(cmd, paths)
			if authErr != nil {
				return authErr
			}
			var oldAPIBaseURL string
			_, hasStoredAuth := authFile.Profiles[name]
			if hasStoredAuth {
				oldView, _, viewErr := config.ResolveProfileView(file, name)
				if viewErr != nil {
					return viewErr
				}
				oldAPIBaseURL = oldView.APIBaseURL
			}
			var rederived bool
			rederivedAPIBaseURL, rederived, err = config.SetProfileBaseURL(file, name, value)
			if err != nil {
				return err
			}
			if hasStoredAuth {
				updatedView, _, viewErr := config.ResolveProfileView(file, name)
				if viewErr != nil {
					return viewErr
				}
				if oldAPIBaseURL != updatedView.APIBaseURL {
					return destinationChangeError(name)
				}
			}
			if !rederived {
				rederivedAPIBaseURL = ""
			}
		case "api_base_url":
			// Resolve the old value only when a credential needs protection.
			// Without stored auth, setting a valid value must remain able to
			// repair an invalid hand-edited api_base_url.
			authFile, authErr := loadAuth(cmd, paths)
			if authErr != nil {
				return authErr
			}
			var oldAPIBaseURL string
			_, hasStoredAuth := authFile.Profiles[name]
			if hasStoredAuth {
				oldView, _, viewErr := config.ResolveProfileView(file, name)
				if viewErr != nil {
					return viewErr
				}
				oldAPIBaseURL = oldView.APIBaseURL
			}
			if err := config.SetProfileAPIBaseURL(file, name, value); err != nil {
				return err
			}
			if hasStoredAuth {
				updatedView, _, viewErr := config.ResolveProfileView(file, name)
				if viewErr != nil {
					return viewErr
				}
				if oldAPIBaseURL != updatedView.APIBaseURL {
					return destinationChangeError(name)
				}
			}
		case "locale":
			if err := config.SetProfileLocale(file, name, value); err != nil {
				return err
			}
		case "defaults.project_list_limit":
			if err := config.SetProfileProjectListLimit(file, name, value); err != nil {
				return err
			}
		case "default_output":
			// default_output is readable for diagnostics, but persisted output
			// defaults are intentionally table-only in the current config contract.
			return &usageError{detail: "profiles." + name + ".default_output is not settable; persisted default output is table-only"}
		default:
			return &usageError{detail: "unsupported config key " + strconv.Quote(key)}
		}
	}

	if err := config.Write(paths.ConfigPath, file); err != nil {
		return err
	}
	updated, err := config.GetValue(file, key)
	if err != nil {
		return err
	}
	notes := []output.Node{}
	if rederivedAPIBaseURL != "" {
		notes = append(notes, output.Guidance("Re-derived api_base_url to "+rederivedAPIBaseURL+"."))
	}
	summary := output.MutationSummary{
		Action: "Updated",
		Name:   key,
		Fields: []output.Node{
			output.Field("value", configValueDetailCell(updated.Value)),
		},
		Notes: notes,
	}
	return f.WriteResult(cmd, nil, cmdutil.HumanOutput{
		Render: mutationHuman(summary),
		Plain:  func(data, prose io.Writer) { mutationPlain(summary)(data, prose) },
	})
}

func destinationChangeError(name string) error {
	return &usageError{detail: "refusing to change the effective API destination for profile " + strconv.Quote(name) + " while it has a stored credential; run chab logout --profile " + name + ", update the URL, then authenticate again with chab login --profile " + name}
}

func secretLikeConfigKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "api_key") || strings.Contains(lower, "secret")
}

const configSetLong = `Set one supported non-secret CLI configuration value.

Supported writable keys are current_profile, profiles.<name>.base_url,
profiles.<name>.api_base_url, profiles.<name>.locale, and
profiles.<name>.defaults.project_list_limit. The target profile must already
exist; this command never creates profiles. Use an empty value for locale to
persist an unset Accept-Language preference.

Stored credentials are not part of config.yml. Secret-bearing keys are refused
locally; manage credentials with chab login and chab logout.

profiles.<name>.default_output is readable with config get and config list, but
is not writable in this config contract.

This command has no stable JSON shape, so --jq and --template are not supported.

Related commands:
  chab config get
  chab profile create`

const configSetExample = `  chab config set current_profile staging
  chab config set profiles.staging.base_url https://staging.example.test
  chab config set profiles.staging.locale ""
  chab config set profiles.staging.defaults.project_list_limit 50 --plain`
