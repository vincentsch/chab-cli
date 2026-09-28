package authcmd

import (
	"errors"
	"io"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/readservice"
)

type whoamiJSON = readservice.Whoami

// missingCredentialGuidanceError preserves the underlying auth exit code while
// adding command-specific guidance to the stderr message.
type missingCredentialGuidanceError struct {
	err error
}

func (e *missingCredentialGuidanceError) Error() string {
	return e.err.Error() + `; run "chab login" or set CHAB_API_KEY`
}

func (e *missingCredentialGuidanceError) Unwrap() error {
	return e.err
}

// NewWhoamiCommand builds chab whoami.
func NewWhoamiCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated team or guest principal",
		Long: `Show the authenticated team token or guest trial principal for the active profile.

Authentication uses CHAB_API_KEY when set, otherwise the stored login for the
selected profile. Missing credentials exit 3 with stdout empty.

Team API keys are created and revoked in the product web app.

JSON output is the flat GET /v1/me context. Team tokens include team_id,
token_public_id, scopes, and token_controls. Guest credentials include
principal_id, guest_id, scopes, credential and free_access; team_id is null.

Use --plain for copy-safe detail rows. Use --jq or --template to reshape the
same stable JSON bytes that --json emits.

Related commands:
  chab login
  chab auth status
  chab doctor`,
		Example: `  chab whoami
  chab whoami --json
  chab whoami --plain
  chab whoami --jq '.team | .name'
  chab whoami --template '{{.team.name}}'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWhoami(cmd, f)
		},
	}
}

// runWhoami is a read command: it resolves credentials, performs the normal
// retrying whoami request, and renders only the response projection.
func runWhoami(cmd *cobra.Command, f *cmdutil.Factory) error {
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return err
	}
	cred, findings, err := f.Credential(rt)
	if err != nil {
		return &missingCredentialGuidanceError{err: err}
	}
	if cred.Source == auth.SourceAuthFile {
		cmdutil.WarnPermissionFindings(cmd.ErrOrStderr(), findings)
	}
	client, err := f.APIClient(rt, cred, cmd)
	if err != nil {
		return err
	}
	data, _, err := client.Whoami(cmd.Context())
	if err != nil {
		return err
	}
	return f.WriteCommandResult(cmd, cmdutil.CommandResult{
		Machine: whoamiProjection(data),
		Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
			renderWhoamiHuman(w, data)
		}, Plain: func(dataW, prose io.Writer) {
			renderWhoamiPlain(dataW, prose, data)
		}},
		Supports: cmdutil.OutputSupport{Human: true, JSON: true, Plain: true, JQ: true, Template: true},
	})
}

// whoamiProjection narrows the API response to the documented public fields.
func whoamiProjection(data api.WhoamiData) whoamiJSON {
	return readservice.WhoamiProjection(data)
}

func (e *missingCredentialGuidanceError) ExitCode() int {
	type exitCoder interface{ ExitCode() int }
	var coded exitCoder
	if errors.As(e.err, &coded) {
		return coded.ExitCode()
	}
	return 3
}
