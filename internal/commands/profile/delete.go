package profile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
)

// NewDeleteCommand builds chab profile delete <name>.
func NewDeleteCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "delete <name>",
		Short:   "Delete a profile",
		Long:    profileDeleteLong,
		Args:    cobra.ExactArgs(1),
		Example: profileDeleteExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileDelete(cmd, f, args[0])
		},
	}
}

// runProfileDelete removes local profile state in the order that preserves the
// clearest failure mode: decide the config outcome, confirm once, remove auth,
// then update or remove config.
func runProfileDelete(cmd *cobra.Command, f *cmdutil.Factory, name string) error {
	if err := config.ValidateProfileName(name); err != nil {
		return err
	}
	file, authFile, paths, err := loadConfigAndAuth(cmd, f)
	if err != nil {
		return err
	}
	_, inConfig := file.Profiles[name]
	_, inAuth := authFile.Profiles[name]
	if !inConfig && !inAuth {
		return &usageError{detail: "profile " + strconv.Quote(name) + " does not exist"}
	}

	var decision config.DeleteProfileResult
	if inConfig {
		// DeleteProfile mutates only memory. Work out the config outcome before
		// prompting so a final-profile delete that would discard user data fails
		// without asking for confirmation.
		decision, err = config.DeleteProfile(file, name)
		if err != nil {
			return err
		}
		if decision.Disposition == config.DispositionKeepFinalProfile {
			return &usageError{detail: "deleting profile " + strconv.Quote(name) + " would remove the last profile and discard custom config data (comments or unknown keys); choose or create another profile first"}
		}
	}

	if err := cmdutil.ConfirmDestructive(f.Prompt(cmd), deleteQuestion(name, inConfig, inAuth, decision)); err != nil {
		return err
	}

	// Remove auth first. If auth cleanup fails, config stays untouched; if the
	// later config write fails, the error can tell the user exactly which local
	// credential was already removed.
	authRemoved := false
	if inAuth {
		if err := auth.DeleteProfile(authFile, name); err != nil {
			return err
		}
		if err := auth.Write(paths.AuthPath, authFile); err != nil {
			return err
		}
		authRemoved = true
	}

	if inConfig {
		var configErr error
		switch decision.Disposition {
		case config.DispositionWrite:
			configErr = config.Write(paths.ConfigPath, file)
		case config.DispositionRemoveFile:
			// DeleteProfile only chooses removal for a final profile in a file
			// with no comments or unknown top-level keys.
			configErr = os.Remove(paths.ConfigPath)
			if errors.Is(configErr, os.ErrNotExist) {
				configErr = nil
			}
		}
		if configErr != nil {
			if authRemoved {
				return &usageError{detail: fmt.Sprintf("profile %q still exists in config, but its local stored credential was removed; recreate it with chab login --profile %q (config write failed: %v)", name, name, configErr)}
			}
			return configErr
		}
	}

	notes := []output.Node{}
	if decision.CurrentChanged {
		notes = append(notes, output.Guidance("Current profile is now "+strconv.Quote(decision.NewCurrent)+"."))
	}
	if inConfig && decision.Disposition == config.DispositionRemoveFile {
		notes = append(notes, output.Guidance("No profiles remain; removed the CLI config file."))
	}
	if authRemoved {
		notes = append(notes, output.Guidance("Removed the locally stored API key for this profile."))
	}
	notes = append(notes, output.Guidance("Server-side API key revocation still happens in the product web app."))

	summary := output.MutationSummary{Action: "Deleted", Resource: "profile", Name: name, Notes: notes}
	return f.WriteResult(cmd, nil, cmdutil.HumanOutput{
		Render: mutationHuman(summary),
		Plain:  func(data, prose io.Writer) { mutationPlain(summary)(data, prose) },
	})
}

// deleteQuestion summarizes exactly which local records will be removed so the
// shared confirmation prompt does not need to know profile-delete internals.
func deleteQuestion(name string, inConfig, inAuth bool, decision config.DeleteProfileResult) string {
	details := []string{}
	if inConfig {
		details = append(details, "its local settings")
	}
	if inAuth {
		details = append(details, "its locally stored API key (not revoked on the server)")
	}
	if decision.CurrentChanged {
		details = append(details, "the current profile will switch to "+strconv.Quote(decision.NewCurrent))
	} else if decision.WasCurrent && decision.RemainingCount == 0 {
		details = append(details, "no profile will be selected")
	}
	if len(details) == 0 {
		return fmt.Sprintf("Delete profile %q?", name)
	}
	return fmt.Sprintf("Delete profile %q? This removes %s.", name, strings.Join(details, "; "))
}

const profileDeleteLong = `Delete a local profile from config.yml and remove its stored credential record when one exists.

This command is local-only. It never revokes a key on the server and never
prints the stored API key. It requires confirmation before any write unless
--yes is supplied. In --no-prompt or non-interactive mode without --yes, it
fails before changing auth.json or config.yml.

When the deleted profile is current, the current profile switches to the first
remaining saved profile in sorted order. If no profiles remain, the CLI-owned
config file is removed. Deleting the last profile fails before confirmation when
removing the file would discard comments or unknown top-level config keys.

This command has no stable JSON shape, so --jq and --template are not supported.

Related commands:
  chab profile list
  chab config path`

const profileDeleteExample = `  chab profile delete staging
  chab profile delete staging --yes
  chab profile delete staging --yes --plain`
