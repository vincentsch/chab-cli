package doctor

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
	"github.com/vincentsch/chab-cli/internal/redact"
)

// NewCommand builds chab doctor.
func NewCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local setup and API readiness",
		Long: `Check local setup and API readiness for common CLI workflows.

Doctor resolves local config and auth paths without creating files. When a
credential exists, it makes at most one GET /v1/me request. Missing
credentials, unsafe auth-file permissions, unreachable API, unusable keys, and
blocked plan API access are warnings. Malformed local config/auth files and
invalid persisted profile values are failing findings.

Stable finding ids are config.path, config.runtime, profile, api.base_url,
locale, auth.path, auth.file, auth.permissions, credential.source,
compatibility, api.connectivity, granted_scopes, token_controls,
spending_allowance, and project_access.

Exit model: 0 means pass or warnings; 1 means one or more failing findings.
The readiness report still renders when doctor exits 1 because of failing
findings. Invocation errors such as invalid flag or environment values keep
stdout empty.

JSON output has exactly the top-level fields status and findings. Each finding
has id, severity, summary, and optional detail and request_id.
Use --plain for a copy-safe TSV table. Use --jq or --template to reshape the
same stable JSON bytes that --json emits.

Related commands:
  chab auth status
  chab login
  chab version`,
		Example: `  chab doctor
  chab doctor --json
  chab doctor --plain
  chab doctor --jq .status
  chab doctor --template '{{.status}}'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd, f)
		},
	}
}

// runDoctor assembles local readiness findings first, then adds a no-retry
// whoami probe only when a credential is available.
func runDoctor(cmd *cobra.Command, f *cmdutil.Factory) error {
	rt, err := f.ResolveRuntime(cmd, config.ResolveStrict)
	if err != nil {
		// Bad flags and env vars are invocation errors. Persisted local config
		// problems become findings only when doctor can still assemble a report.
		if !configErrorIsReadinessFinding(err, cmdutil.RuntimeFlagOverrides(cmd), f) {
			return err
		}
		return renderAndReturn(cmd, f, []finding{{
			ID:       "config.runtime",
			Severity: "fail",
			Summary:  "could not resolve local runtime",
			Detail:   err.Error() + "; remaining checks skipped",
		}})
	}

	findings := localFindings(rt)
	findings = append(findings, compatibilityFinding(cmd, f, rt)...)
	authFile, authFindings, authErr := auth.Load(rt.AuthPath)
	if authErr != nil {
		findings = append(findings,
			finding{ID: "auth.path", Severity: "ok", Summary: "auth path resolved", Detail: fmt.Sprintf("%s (exists: unknown)", redact.String(rt.AuthPath))},
			finding{ID: "auth.file", Severity: "fail", Summary: "could not read auth file", Detail: authErr.Error()},
		)
		return renderAndReturn(cmd, f, findings)
	}
	findings = append(findings,
		finding{ID: "auth.path", Severity: "ok", Summary: "auth path resolved", Detail: fmt.Sprintf("%s (exists: %t)", redact.String(rt.AuthPath), authFile.Exists())},
		finding{ID: "auth.file", Severity: "ok", Summary: "auth file readable", Detail: fmt.Sprintf("exists: %t", authFile.Exists())},
	)

	if len(authFindings) > 0 {
		for _, p := range authFindings {
			findings = append(findings, finding{
				ID:       "auth.permissions",
				Severity: "warning",
				Summary:  "auth file permissions are broader than expected",
				Detail:   fmt.Sprintf("%s has permissions %04o; expected %04o. Tighten it with: chmod %03o %s", p.Path, p.ActualMode.Perm(), p.ExpectedMode.Perm(), p.ExpectedMode.Perm(), p.Path),
			})
		}
	} else if authFile.Exists() {
		findings = append(findings, finding{ID: "auth.permissions", Severity: "ok", Summary: "auth file permissions are restricted"})
	}

	state, err := f.CredentialState(rt, true)
	if err != nil {
		findings = append(findings, finding{ID: "auth.file", Severity: "fail", Summary: "could not inspect auth file", Detail: err.Error()})
		return renderAndReturn(cmd, f, findings)
	}
	if state.Source == "" {
		// Doctor is a local readiness report as well as a live API check, so a
		// missing credential is a warning and simply skips the whoami probe.
		findings = append(findings, finding{
			ID:       "credential.source",
			Severity: "warning",
			Summary:  "no credential found",
			Detail:   "run chab login or set CHAB_API_KEY",
		})
		return renderAndReturn(cmd, f, findings)
	}
	findings = append(findings, credentialFinding(state))

	client, err := f.APIClientNoRetry(rt, state.Credential, cmd)
	if err != nil {
		findings = append(findings, finding{ID: "api.connectivity", Severity: "warning", Summary: "could not build API client", Detail: err.Error()})
		return renderAndReturn(cmd, f, findings)
	}
	data, meta, err := client.Whoami(cmd.Context())
	if err != nil {
		findings = append(findings, apiErrorFinding(err)...)
		return renderAndReturn(cmd, f, findings)
	}
	findings = append(findings, successAPIFindings(data, meta)...)
	return renderAndReturn(cmd, f, findings)
}

// localFindings records resolved local values without requiring credentials or
// network access. It reloads config only to report whether the file currently
// exists.
func localFindings(rt config.Runtime) []finding {
	cfg, err := config.Load(rt.ConfigPath)
	exists := false
	if err == nil {
		exists = cfg.Exists()
	}
	locale := rt.Locale
	if locale == "" {
		locale = "not set"
	}
	return []finding{
		{ID: "config.path", Severity: "ok", Summary: "config path resolved", Detail: fmt.Sprintf("%s (exists: %t)", redact.String(rt.ConfigPath), exists)},
		{ID: "config.runtime", Severity: "ok", Summary: "runtime resolved"},
		{ID: "profile", Severity: "ok", Summary: "profile selected", Detail: rt.Profile},
		{ID: "api.base_url", Severity: "ok", Summary: "API base URL resolved", Detail: rt.APIBaseURL},
		{ID: "locale", Severity: "ok", Summary: "locale resolved", Detail: locale},
	}
}

// credentialFinding reports where the selected credential came from without
// exposing the key material.
func credentialFinding(state cmdutil.CredentialState) finding {
	detail := string(state.Source)
	if state.Source == auth.SourceAuthFile && state.Record != nil && state.Record.TokenPublicID != "" {
		detail += " (" + state.Record.TokenPublicID + ")"
	} else if state.Source == auth.SourceAuthFile && state.Record != nil && state.Record.PrincipalType == "guest_trial" {
		detail += " (guest principal " + state.Record.PrincipalID + ")"
	}
	return finding{ID: "credential.source", Severity: "ok", Summary: "credential found", Detail: detail}
}

func compatibilityFinding(cmd *cobra.Command, f *cmdutil.Factory, rt config.Runtime) []finding {
	bootstrap, err := f.BootstrapClient(rt, cmd)
	if err != nil {
		return []finding{{ID: "compatibility", Severity: "warning", Summary: "could not build compatibility client", Detail: err.Error()}}
	}
	version := api.EffectiveClientVersion(f.VersionString())
	data, meta, err := bootstrap.Compatibility(cmd.Context(), version)
	if err != nil {
		return []finding{{ID: "compatibility", Severity: "warning", Summary: "compatibility check failed", Detail: err.Error()}}
	}
	detail := fmt.Sprintf("client %s, api_major %d, catalog %s", version, data.APIMajor, data.CatalogVersion)
	return []finding{{ID: "compatibility", Severity: "ok", Summary: "CLI compatibility accepted", Detail: detail, RequestID: meta.RequestID}}
}

// successAPIFindings maps a successful whoami response into the small doctor
// finding vocabulary. Successful rate-limit headers are intentionally ignored;
// they are API metadata, not readiness problems.
func successAPIFindings(data api.WhoamiData, meta api.ResponseMeta) []finding {
	findings := []finding{{
		ID:        "api.connectivity",
		Severity:  "ok",
		Summary:   "/me request succeeded",
		Detail:    requestDetail(meta.RequestID),
		RequestID: meta.RequestID,
	}}
	if data.PrincipalType == "guest_trial" {
		return append(findings,
			finding{ID: "granted_scopes", Severity: "ok", Summary: "guest scopes reported", Detail: redact.String(strings.Join(data.Scopes, " "))},
			finding{ID: "guest_principal", Severity: "ok", Summary: "guest trial principal reported", Detail: redact.String(data.PrincipalID)},
		)
	}
	findings = append(findings,
		finding{ID: "granted_scopes", Severity: "ok", Summary: "granted scopes reported", Detail: redact.String(strings.Join(data.Scopes, " "))},
		finding{ID: "token_controls", Severity: "ok", Summary: "token controls reported", Detail: fmt.Sprintf("policy revision %d; feature access %s", data.TokenControls.PolicyRevision, redact.String(data.TokenControls.FeatureAccess.Mode))},
		finding{ID: "spending_allowance", Severity: "ok", Summary: "spending allowance reported", Detail: spendingDetail(data.TokenControls.Spending)},
		finding{ID: "project_access", Severity: "ok", Summary: "project access reported", Detail: projectAccessDetail(data.TokenControls.ProjectAccess)},
	)
	return findings
}

// apiErrorFinding converts whoami probe failures into warning findings instead
// of returning the API client's normal process exit categories.
func apiErrorFinding(err error) []finding {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		id := "api.connectivity"
		summary := "API returned " + apiErr.Code
		detail := apiErr.Error()
		switch apiErr.Code {
		case "api_token_missing", "invalid_api_token", "api_token_expired", "api_token_revoked", "team_token_required", "access_denied":
			id = "credential.source"
			summary = "credential is not usable"
			detail += "; create or rotate an API key in the product web app"
		case "guest_credential_expired", "guest_credential_revoked":
			id = "credential.source"
			summary = "guest trial credential is not usable"
			detail += "; issue a new credential in the browser trial UI if eligible"
		case "signup_required":
			id = "credential.source"
			summary = "guest trial has been claimed"
			detail += "; sign in and use a team API key"
		case "api_access_not_included", "subscription_unhealthy", "paid_plan_required":
			id = "token_controls"
			summary = "API access is unavailable"
		case "api_scope_missing":
			id = "granted_scopes"
			summary = "required scope is missing"
		case "token_spending_disabled", "token_budget_exceeded", "max_budget_exceeded", "insufficient_credits":
			id = "spending_allowance"
			summary = "spending allowance is unavailable"
		case "rate_limited":
			id = "api.connectivity"
			summary = "API rate limit reached"
			if apiErr.Meta.RetryAfter.Raw != "" {
				detail += "; retry-after: " + apiErr.Meta.RetryAfter.Raw
			}
		}
		return []finding{{ID: id, Severity: "warning", Summary: summary, Detail: detail, RequestID: apiErr.RequestID}}
	}
	var protocolErr *api.ProtocolError
	if errors.As(err, &protocolErr) {
		return []finding{{ID: "api.connectivity", Severity: "warning", Summary: "API response could not be decoded", Detail: protocolErr.Error(), RequestID: protocolErr.RequestID}}
	}
	return []finding{{ID: "api.connectivity", Severity: "warning", Summary: "could not complete whoami request", Detail: err.Error()}}
}

// renderAndReturn always renders assembled readiness findings. A non-nil error
// is returned only after rendering, and only when at least one finding failed.
func renderAndReturn(cmd *cobra.Command, f *cmdutil.Factory, findings []finding) error {
	r, failures := finishReport(findings)
	if err := f.WriteCommandResult(cmd, cmdutil.CommandResult{
		Machine: r,
		Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
			renderHuman(w, r)
		}, Plain: func(data, prose io.Writer) {
			renderPlain(data, prose, r)
		}},
		Supports: cmdutil.OutputSupport{Human: true, JSON: true, Plain: true, JQ: true, Template: true},
	}); err != nil {
		return err
	}
	if failures > 0 {
		return failError{count: failures}
	}
	return nil
}

// configErrorIsReadinessFinding separates invalid persisted local state from
// bad invocation input. The former is useful in a doctor report; the latter
// should stay a normal stderr-only command error.
func configErrorIsReadinessFinding(err error, flags config.FlagOverrides, f *cmdutil.Factory) bool {
	var cfgErr *config.Error
	if !errors.As(err, &cfgErr) {
		return false
	}
	switch cfgErr.Kind {
	case config.ErrMissingProfile, config.ErrUnsupportedVersion:
		return true
	case config.ErrMalformedConfig:
		return cfgErr.Field != "config" && cfgErr.Field != "auth_file" && cfgErr.Field != "user_config_dir"
	case config.ErrInvalidDefault:
		return true
	case config.ErrInvalidURL, config.ErrInvalidLocale, config.ErrInvalidProfileName:
		return persistedWinningValue(cfgErr.Field, flags, f)
	default:
		return false
	}
}

// persistedWinningValue reports whether the invalid value came from stored
// config instead of a flag or environment override. Env detection matches
// config resolution: any non-empty string is supplied, even whitespace.
func persistedWinningValue(field string, flags config.FlagOverrides, f *cmdutil.Factory) bool {
	switch field {
	case "base_url":
		return !flags.BaseURL.Set && !envNonBlank(f, "CHAB_BASE_URL")
	case "api_base_url":
		return !flags.APIBaseURL.Set && !envNonBlank(f, "CHAB_API_BASE_URL")
	case "locale":
		return !flags.Locale.Set && !envNonBlank(f, "CHAB_LOCALE")
	case "profile":
		return !flags.Profile.Set && !envNonBlank(f, "CHAB_PROFILE")
	default:
		return true
	}
}

func envNonBlank(f *cmdutil.Factory, name string) bool {
	if f == nil || f.LookupEnv == nil {
		return false
	}
	value, ok := f.LookupEnv(name)
	return ok && value != ""
}

func requestDetail(requestID string) string {
	if requestID == "" {
		return "request completed"
	}
	return "request id " + requestID
}

func spendingDetail(spending api.SpendingAccess) string {
	parts := []string{spending.Mode, fmt.Sprintf("active reserved %d", spending.ActiveReservedCredits)}
	if spending.Allowance != nil {
		parts = append(parts,
			fmt.Sprintf("remaining %d", spending.Allowance.RemainingCredits),
			fmt.Sprintf("limit %d", spending.Allowance.LimitCredits),
		)
	}
	return redact.String(strings.Join(parts, "; "))
}

func projectAccessDetail(access api.ProjectAccess) string {
	parts := []string{access.Mode, fmt.Sprintf("%d selected", access.SelectedCount)}
	if len(access.SelectedProjectIDs) > 0 {
		ids := make([]string, 0, len(access.SelectedProjectIDs))
		for _, id := range access.SelectedProjectIDs {
			ids = append(ids, fmt.Sprintf("%d", id))
		}
		parts = append(parts, strings.Join(ids, ", "))
	}
	return redact.String(strings.Join(parts, "; "))
}
