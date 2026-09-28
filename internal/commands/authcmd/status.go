package authcmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/redact"
)

// statusReport is the stable JSON shape for auth status. Stored and live are
// pointers so JSON can distinguish "not applicable/not run" from an empty
// object.
type statusReport struct {
	Profile          string      `json:"profile"`
	APIBaseURL       string      `json:"api_base_url"`
	CredentialSource *string     `json:"credential_source"`
	Usable           bool        `json:"usable"`
	Stored           *storedMeta `json:"stored"`
	Live             *liveState  `json:"live"`
	RequestID        string      `json:"request_id,omitempty"`
}

// storedMeta is the narrowed, non-secret subset copied from auth.json. It
// deliberately omits the API key and any unknown auth-file fields.
type storedMeta struct {
	PrincipalType   string   `json:"principal_type,omitempty"`
	PrincipalID     string   `json:"principal_id,omitempty"`
	TeamID          int64    `json:"team_id,omitempty"`
	TokenPublicID   string   `json:"token_public_id,omitempty"`
	TokenID         string   `json:"token_id,omitempty"`
	Scopes          []string `json:"scopes,omitempty"`
	LastValidatedAt *string  `json:"last_validated_at"`
}

// liveState is filled only when the one-request whoami probe runs. Failed
// auth/authorization probes keep just the API error code and optional request
// id so stdout can stay a compact status report.
type liveState struct {
	OK            bool     `json:"ok"`
	ErrorCode     string   `json:"error_code,omitempty"`
	PrincipalType string   `json:"principal_type,omitempty"`
	PrincipalID   string   `json:"principal_id,omitempty"`
	TeamID        int64    `json:"team_id,omitempty"`
	TokenPublicID string   `json:"token_public_id,omitempty"`
	TokenID       string   `json:"token_id,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
}

// statusProbeError remaps not-found responses from the whoami probe into the
// generic API/network bucket. For status, a missing whoami route means the
// readiness check failed, not that a user-requested resource was absent.
type statusProbeError struct {
	err error
}

func (e *statusProbeError) Error() string {
	return "auth status API/network error: " + e.err.Error()
}

func (e *statusProbeError) Unwrap() error {
	return e.err
}

func (e *statusProbeError) ExitCode() int {
	return 2
}

// NewStatusCommand builds chab auth status.
func NewStatusCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report credential and key state",
		Long: `Report credential and key state for the active profile.

The command is read-only. It never creates config or auth files and never
refreshes cached auth metadata. When a credential exists, it makes at most one
GET /v1/me request.

JSON output has exactly these top-level fields: profile, api_base_url,
credential_source, usable, stored, live, and optional request_id. stored is
null for environment or missing credentials. live is null when no live probe
ran. Stored metadata is limited to principal_type, principal_id (for guests),
team_id, token_public_id, token_id, scopes, and last_validated_at.
Use --plain for copy-safe status rows. Use --jq or --template to reshape the
same stable JSON bytes that --json emits.

Exit codes: 0 usable; 3 no credential, invalid, expired, or revoked; 4 plan,
scope, token-control, or authorization denial; 2 API/network/protocol error; 6
rate limited; 1 local usage or config error. Reports render on stdout for exits
0, 3, and 4. Stdout stays empty for exits 1, 2, and 6. Not-found responses from
the /me probe are folded into exit 2.

Team API keys are created and revoked in the product web app.

Related commands:
  chab login
  chab whoami
  chab doctor`,
		Example: `  chab auth status
  chab auth status --json
  chab auth status --plain
  chab auth status --jq .usable
  chab auth status --template '{{.profile}}'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd, f)
		},
	}
}

// runStatus builds a read-only local report, then optionally adds one live
// whoami probe. Only auth and authorization failures render a report before
// returning a non-zero exit error.
func runStatus(cmd *cobra.Command, f *cmdutil.Factory) error {
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		return err
	}
	state, err := f.CredentialState(rt, false)
	if err != nil {
		return err
	}
	if state.Source == auth.SourceAuthFile {
		cmdutil.WarnPermissionFindings(cmd.ErrOrStderr(), state.Findings)
	}

	report := statusReport{
		Profile:    rt.Profile,
		APIBaseURL: rt.APIBaseURL,
	}
	if state.Source != "" {
		source := string(state.Source)
		report.CredentialSource = &source
	}
	if state.Source == auth.SourceAuthFile && state.Record != nil {
		report.Stored = storedMetaFromRecord(*state.Record)
	}

	if state.Source == "" {
		// Missing credentials are still a useful status report, so render first
		// and return the typed auth error for the process exit code.
		if err := writeStatusReport(cmd, f, report, false); err != nil {
			return err
		}
		return missingCredentialError(rt)
	}

	client, err := f.APIClientNoRetry(rt, state.Credential, cmd)
	if err != nil {
		return err
	}
	data, meta, err := client.Whoami(cmd.Context())
	if err == nil {
		report.Usable = true
		report.RequestID = meta.RequestID
		report.Live = &liveState{
			OK:            true,
			PrincipalType: data.PrincipalType,
			PrincipalID:   data.PrincipalID,
			TeamID:        data.TeamID,
			TokenPublicID: data.TokenPublicID,
			TokenID:       data.TokenID,
			Scopes:        append([]string(nil), data.Scopes...),
		}
		return writeStatusReport(cmd, f, report, false)
	}

	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		exit := apiErr.ExitCode()
		if exit == 3 || exit == 4 {
			// Auth and authorization failures are part of the report contract:
			// users need the stored/live comparison plus the API error on stderr.
			report.Live = &liveState{OK: false, ErrorCode: apiErr.Code}
			report.RequestID = apiErr.RequestID
			if err := writeStatusReport(cmd, f, report, exit == 3); err != nil {
				return err
			}
			return err
		}
		if exit == 5 {
			return &statusProbeError{err: err}
		}
	}
	return err
}

func missingCredentialError(rt config.Runtime) error {
	_, _, err := auth.LookupCredential(rt, auth.Options{LookupEnv: func(string) (string, bool) { return "", false }})
	return err
}

func storedMetaFromRecord(record auth.ProfileAuth) *storedMeta {
	return &storedMeta{
		PrincipalType:   record.PrincipalType,
		PrincipalID:     record.PrincipalID,
		TeamID:          record.TeamID,
		TokenPublicID:   record.TokenPublicID,
		TokenID:         record.TokenID,
		Scopes:          append([]string(nil), record.Scopes...),
		LastValidatedAt: formatTimePtr(record.LastValidatedAt),
	}
}

func writeStatusReport(cmd *cobra.Command, f *cmdutil.Factory, report statusReport, includeKeyHint bool) error {
	return f.WriteCommandResult(cmd, cmdutil.CommandResult{
		Machine: report,
		Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
			renderStatusHuman(w, report, includeKeyHint)
		}, Plain: func(data, prose io.Writer) {
			statusDetail(report, includeKeyHint).RenderPlain(data, prose)
		}},
		Supports: cmdutil.OutputSupport{Human: true, JSON: true, Plain: true, JQ: true, Template: true},
	})
}

func renderStatusHuman(w io.Writer, report statusReport, includeKeyHint bool) {
	statusDetail(report, includeKeyHint).Render(w)
}

func statusDetail(report statusReport, includeKeyHint bool) output.Detail {
	source := "none"
	if report.CredentialSource != nil {
		source = *report.CredentialSource
	}
	nodes := []output.Node{
		output.Field("Profile", redact.String(report.Profile)),
		output.Field("API base URL", redact.String(report.APIBaseURL)),
		output.Field("Credential source", redact.String(source)),
		output.Field("Usable", fmt.Sprintf("%t", report.Usable)),
	}
	if report.Stored != nil {
		nodes = append(nodes, output.Field("Stored principal", redact.String(report.Stored.PrincipalType)))
		if report.Stored.PrincipalType == "guest_trial" {
			nodes = append(nodes, output.Field("Stored principal ID", redact.String(report.Stored.PrincipalID)))
		} else {
			nodes = append(nodes,
				output.Field("Stored team ID", fmt.Sprintf("%d", report.Stored.TeamID)),
				output.Field("Stored token", redact.String(report.Stored.TokenPublicID)),
			)
		}
		if len(report.Stored.Scopes) > 0 {
			nodes = append(nodes, output.Field("Stored scopes", redact.String(strings.Join(report.Stored.Scopes, " "))))
		}
		if report.Stored.LastValidatedAt != nil {
			nodes = append(nodes, output.Field("Last validated", *report.Stored.LastValidatedAt))
		}
	}
	if report.Live == nil {
		nodes = append(nodes, output.Field("Live check", "not run"))
	} else if report.Live.OK {
		if report.Live.PrincipalType == "guest_trial" {
			nodes = append(nodes, output.Field("Live check", "OK for guest principal "+redact.String(report.Live.PrincipalID)))
		} else {
			nodes = append(nodes, output.Field("Live check", fmt.Sprintf("OK for team %d token %s", report.Live.TeamID, redact.String(report.Live.TokenPublicID))))
		}
	} else {
		nodes = append(nodes, output.Field("Live check", fmt.Sprintf("failed (%s)", redact.String(report.Live.ErrorCode))))
		if report.RequestID != "" {
			nodes = append(nodes, output.Field("Request ID", redact.String(report.RequestID)))
		}
	}
	if includeKeyHint {
		nodes = append(nodes, output.Guidance("Create or rotate an API key in the product web app, then run chab login."))
	}
	return output.Detail{Nodes: nodes}
}
