package authcmd

import (
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
)

// NewLogoutCommand builds chab auth logout.
func NewLogoutCommand(f *cmdutil.Factory) *cobra.Command {
	return newLogoutCommand(f, "logout", logoutLong(false), "auth logout")
}

// NewLogoutAlias builds chab logout.
func NewLogoutAlias(f *cmdutil.Factory) *cobra.Command {
	return newLogoutCommand(f, "logout", logoutLong(true), "logout")
}

func newLogoutCommand(f *cmdutil.Factory, use, long, invocation string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: "Remove the stored credential for the active profile",
		Long:  long,
		Example: fmt.Sprintf(`  chab %s
  chab %s --plain --profile staging`, invocation, invocation),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogout(cmd, f)
		},
	}
}

func logoutLong(alias bool) string {
	text := `Remove the stored credential for the selected profile without changing non-secret configuration.

Logout is local and idempotent. It removes only the selected profile's stored
auth record and cached metadata, preserving other profiles, config, and
current_profile. It does not revoke the API key on the server; revoke keys in
the product web app. If CHAB_API_KEY is set, that environment credential still
takes precedence until it is unset.

Team API keys are created and revoked in the product web app. chab never prints
the stored API key; logout shows only non-secret key metadata.

Output is human-readable even when --json is inherited from the root. Use
--plain for copy-safe local removal rows. jq and template output are not
available because logout has no stable JSON shape.

Related commands:
  chab login
  chab auth status`
	if alias {
		return text + "\n\nThis shorthand mirrors chab auth logout."
	}
	return text
}

func runLogout(cmd *cobra.Command, f *cmdutil.Factory) error {
	rt, err := f.ResolveRuntime(cmd, config.ResolveForWrite)
	if err != nil {
		return err
	}
	file, findings, err := auth.Load(rt.AuthPath)
	cmdutil.WarnPermissionFindings(cmd.ErrOrStderr(), findings)
	if err != nil {
		return err
	}

	record, ok := file.Profiles[rt.Profile]
	if !ok {
		envOverride := envNonBlank(f, "CHAB_API_KEY")
		if err := f.WriteCommandResult(cmd, cmdutil.CommandResult{
			Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
				fmt.Fprintf(w, "Local auth state is already clear for profile %q.\n", auth.RedactString(rt.Profile))
				fmt.Fprintln(w, "Server-side API key revocation happens in the product web app.")
			}, Plain: func(data, prose io.Writer) {
				renderLogoutPlain(data, prose, rt.Profile, false, "", envOverride)
			}},
			Supports: cmdutil.OutputSupport{Human: true, Plain: true},
		}); err != nil {
			return err
		}
		if !cmdutil.PlainEnabled(cmd) {
			warnEnvOverride(cmd, f)
		}
		return nil
	}

	if err := auth.DeleteProfile(file, rt.Profile); err != nil {
		return err
	}
	if err := auth.Write(rt.AuthPath, file); err != nil {
		return err
	}
	envOverride := envNonBlank(f, "CHAB_API_KEY")
	if err := f.WriteCommandResult(cmd, cmdutil.CommandResult{
		Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
			fmt.Fprintf(w, "Removed stored credential for profile %q", auth.RedactString(rt.Profile))
			if record.DisplayID != "" {
				fmt.Fprintf(w, " (%s)", auth.RedactString(record.DisplayID))
			}
			fmt.Fprintln(w, ".")
			fmt.Fprintln(w, "Removal is local only; revoke the key itself in the product web app.")
		}, Plain: func(data, prose io.Writer) {
			renderLogoutPlain(data, prose, rt.Profile, true, record.DisplayID, envOverride)
		}},
		Supports: cmdutil.OutputSupport{Human: true, Plain: true},
	}); err != nil {
		return err
	}
	if !cmdutil.PlainEnabled(cmd) {
		warnEnvOverride(cmd, f)
	}
	return nil
}

func renderLogoutPlain(data, prose io.Writer, profile string, removed bool, keyDisplayID string, envOverride bool) {
	nodes := []output.Node{
		output.Field("profile", auth.RedactString(profile)),
		output.Field("removed", strconv.FormatBool(removed)),
	}
	if keyDisplayID != "" {
		nodes = append(nodes, output.Field("key_display_id", auth.RedactString(keyDisplayID)))
	}
	if envOverride {
		nodes = append(nodes, output.Field("env_override", "true"))
	}
	nodes = append(nodes, output.Guidance("Server-side API key revocation happens in the product web app."))
	if envOverride {
		nodes = append(nodes, output.Guidance("Warning: CHAB_API_KEY is still set and takes precedence over local auth state."))
	}
	output.Detail{Nodes: nodes}.RenderPlain(data, prose)
}

func warnEnvOverride(cmd *cobra.Command, f *cmdutil.Factory) {
	if envNonBlank(f, "CHAB_API_KEY") {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: CHAB_API_KEY is still set and takes precedence over local auth state.")
	}
}
