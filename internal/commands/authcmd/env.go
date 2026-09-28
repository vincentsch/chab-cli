package authcmd

import (
	"io"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/readservice"
)

const envSecretVariable = readservice.SecretVariableName

type envReport = readservice.AuthEnvReport

// NewEnvCommand reports the effective non-secret runtime for CI use.
// Execution deliberately stops after strict runtime resolution and rendering.
func NewEnvCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "Report the effective authentication environment",
		Long: `Report the effective, non-secret authentication environment for this invocation.

The command uses strict runtime resolution, including normal flag, environment,
profile-file, normalization, and API-base derivation rules. This differs from
profile show and config inspection, which report persisted profile-file state
without applying all invocation overrides.

JSON output is one stable object with exactly profile, base_url, api_base_url,
locale, and secret_variable. locale remains present as an empty string when
unset. Use --plain for ordered CHAB_* tab-separated rows; its CHAB_LOCALE row
is omitted when locale is unset. Use --jq or --template to reshape the same
stable JSON value.

This is not an authentication or connectivity check. It may inspect whether
the selected profile has a stored auth-file key but no saved destination, so a
legacy auth-only profile keeps its localhost destination; the key is never
returned. It does not authenticate, construct an API client, or contact the
network. Set CHAB_API_KEY from your CI provider's secret store.
Team API keys are created and revoked in the product web app.

Copying both URL rows freezes the resolved API base URL. A later change to only
CHAB_BASE_URL will not re-derive it while CHAB_API_BASE_URL remains explicitly
set.

Related commands:
  chab profile show
  chab config list
  chab auth status`,
		Example: `  chab auth env
  chab auth env --profile staging --plain
  chab auth env --profile staging --json
  chab auth env --profile staging --jq .api_base_url
  chab auth env --profile staging --template '{{.profile}}'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
			if err != nil {
				return err
			}
			report := readservice.AuthEnv(rt)
			// Machine output and both display trees consume the same report.
			// Human and plain stay separate so headings cannot become TSV rows.
			return f.WriteCommandResult(cmd, cmdutil.CommandResult{
				Machine: report,
				Human: cmdutil.HumanOutput{
					Render: func(w io.Writer) {
						envHumanDetail(report).Render(w)
					},
					Plain: envPlainDetail(report).RenderPlain,
				},
				Supports: cmdutil.OutputSupport{
					Human:    true,
					JSON:     true,
					Plain:    true,
					JQ:       true,
					Template: true,
				},
			})
		},
	}
}

// envHumanDetail builds the labeled, reader-oriented report.
func envHumanDetail(report envReport) output.Detail {
	nodes := []output.Node{
		output.Line("CI authentication environment"),
		output.Field("Profile", report.Profile),
		output.Field("Base URL", report.BaseURL),
		output.Field("API base URL", report.APIBaseURL),
	}
	if report.Locale != "" {
		nodes = append(nodes, output.Field("Locale", report.Locale))
	}
	nodes = append(nodes, output.Guidance("Set CHAB_API_KEY from your CI provider's secret store."))
	return output.Detail{Nodes: nodes}
}

// envPlainDetail builds only copy-safe environment rows and routes guidance
// through the plain renderer's stderr channel.
func envPlainDetail(report envReport) output.Detail {
	nodes := []output.Node{
		output.Field("CHAB_PROFILE", report.Profile),
		output.Field("CHAB_BASE_URL", report.BaseURL),
		output.Field("CHAB_API_BASE_URL", report.APIBaseURL),
	}
	if report.Locale != "" {
		nodes = append(nodes, output.Field("CHAB_LOCALE", report.Locale))
	}
	nodes = append(nodes, output.Guidance("Set CHAB_API_KEY from your CI provider's secret store."))
	return output.Detail{Nodes: nodes}
}
