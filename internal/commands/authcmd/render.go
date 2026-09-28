package authcmd

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/redact"
)

func renderLoginSuccess(w io.Writer, profile, apiBaseURL, locale string, data api.WhoamiData) {
	loginSuccessDetail(profile, apiBaseURL, locale, data).Render(w)
}

func renderLoginPlain(dataW io.Writer, prose io.Writer, profile, apiBaseURL, locale string, data api.WhoamiData) {
	loginSuccessPlainDetail(profile, apiBaseURL, locale, data).RenderPlain(dataW, prose)
}

func renderSetupSuccess(w io.Writer, profile, apiBaseURL string, data api.WhoamiData) {
	setupSuccessDetail(profile, apiBaseURL, data).Render(w)
}

func renderSetupPlain(dataW io.Writer, prose io.Writer, profile, apiBaseURL string, data api.WhoamiData) {
	setupSuccessPlainDetail(profile, apiBaseURL, data).RenderPlain(dataW, prose)
}

func renderWhoamiHuman(w io.Writer, data api.WhoamiData) {
	identityDetail(data).Render(w)
}

func renderWhoamiPlain(dataW io.Writer, prose io.Writer, data api.WhoamiData) {
	identityDetail(data).RenderPlain(dataW, prose)
}

// loginSuccessDetail preserves the compact prose-oriented shape used by the
// default login success message.
func loginSuccessDetail(profile, apiBaseURL, locale string, data api.WhoamiData) output.Detail {
	nodes := []output.Node{
		output.Line("Logged in."),
		output.Field("Profile", redact.String(profile)),
	}
	nodes = append(nodes, identityNodes(data)...)
	nodes = append(nodes, output.Field("API base URL", redact.String(apiBaseURL)))
	if locale != "" {
		nodes = append(nodes, output.Field("Locale", redact.String(locale)))
	}
	return output.Detail{Nodes: nodes}
}

// loginSuccessPlainDetail uses lowercase dotted keys so scripts can read login
// metadata without parsing the human identity summary.
func loginSuccessPlainDetail(profile, apiBaseURL, locale string, data api.WhoamiData) output.Detail {
	nodes := []output.Node{
		output.Field("profile", redact.String(profile)),
		output.Field("api_base_url", redact.String(apiBaseURL)),
		output.Field("principal_type", redact.String(data.PrincipalType)),
	}
	if data.PrincipalType == "guest_trial" {
		nodes = append(nodes, output.Field("principal_id", redact.String(data.PrincipalID)))
	} else {
		nodes = append(nodes,
			output.Field("team_id", strconv.FormatInt(data.TeamID, 10)),
			output.Field("token_public_id", redact.String(data.TokenPublicID)),
		)
	}
	nodes = append(nodes, output.Field("scopes", redact.String(strings.Join(data.Scopes, " "))))
	if locale != "" {
		nodes = append(nodes, output.Field("locale", redact.String(locale)))
	}
	nodes = append(nodes,
		output.Field("stored", "true"),
		output.Field("authenticated", "true"),
	)
	return output.Detail{Nodes: nodes}
}

// setupSuccessDetail is deliberately smaller than the full login identity
// summary: it reports only the fields needed to confirm a ready profile.
func setupSuccessDetail(profile, apiBaseURL string, data api.WhoamiData) output.Detail {
	if data.PrincipalType == "guest_trial" {
		return output.Detail{Nodes: []output.Node{
			output.Line("Guest trial setup complete."),
			output.Field("Profile", redact.String(profile)),
			output.Field("API base URL", redact.String(apiBaseURL)),
			output.Field("Principal ID", redact.String(data.PrincipalID)),
			output.Field("Credential source", string(auth.SourceAuthFile)),
			output.Field("Ready", "true"),
		}}
	}
	return output.Detail{Nodes: []output.Node{
		output.Line("Setup complete."),
		output.Field("Profile", redact.String(profile)),
		output.Field("API base URL", redact.String(apiBaseURL)),
		output.Field("Team ID", strconv.FormatInt(data.TeamID, 10)),
		output.Field("Token public ID", redact.String(data.TokenPublicID)),
		output.Field("Credential source", string(auth.SourceAuthFile)),
		output.Field("Ready", "true"),
	}}
}

// setupSuccessPlainDetail uses sections that flatten into the documented seven
// dotted-key rows without adding prose to stdout.
func setupSuccessPlainDetail(profile, apiBaseURL string, data api.WhoamiData) output.Detail {
	if data.PrincipalType == "guest_trial" {
		return output.Detail{Nodes: []output.Node{
			output.Field("profile", redact.String(profile)),
			output.Field("api_base_url", redact.String(apiBaseURL)),
			output.Field("principal_type", "guest_trial"),
			output.Field("principal_id", redact.String(data.PrincipalID)),
			output.Field("credential_source", string(auth.SourceAuthFile)),
			output.Field("ready", "true"),
		}}
	}
	return output.Detail{Nodes: []output.Node{
		output.Field("profile", redact.String(profile)),
		output.Field("api_base_url", redact.String(apiBaseURL)),
		output.Field("team_id", strconv.FormatInt(data.TeamID, 10)),
		output.Field("token_public_id", redact.String(data.TokenPublicID)),
		output.Field("credential_source", string(auth.SourceAuthFile)),
		output.Field("ready", "true"),
	}}
}

// identityDetail is shared by login success and whoami so both commands
// describe the validated team, key, plan, capabilities, and project scope the
// same way.
func identityDetail(data api.WhoamiData) output.Detail {
	return output.Detail{Nodes: identityNodes(data)}
}

func identityNodes(data api.WhoamiData) []output.Node {
	if data.PrincipalType == "guest_trial" {
		return []output.Node{
			output.Field("Principal", "guest_trial"),
			output.Field("Principal ID", redact.String(data.PrincipalID)),
			output.Field("Guest ID", redact.String(data.GuestID)),
			output.Field("Scopes", redact.String(strings.Join(data.Scopes, " "))),
		}
	}
	nodes := []output.Node{
		output.Field("Principal", redact.String(data.PrincipalType)),
		output.Field("Team ID", strconv.FormatInt(data.TeamID, 10)),
		output.Field("Token public ID", redact.String(data.TokenPublicID)),
		output.Field("Token ID", redact.String(data.TokenID)),
		output.Field("Scopes", redact.String(strings.Join(data.Scopes, " "))),
		output.Field("Feature access", redact.String(data.TokenControls.FeatureAccess.Mode)),
		output.Field("Project access", projectAccessSummary(data.TokenControls.ProjectAccess)),
		output.Field("Spending", spendingSummary(data.TokenControls.Spending)),
		output.Field("IP restrictions", ipRestrictionSummary(data.TokenControls.IPRestrictions)),
		output.Field("Policy revision", strconv.Itoa(data.TokenControls.PolicyRevision)),
	}
	return nodes
}

func projectAccessSummary(access api.ProjectAccess) string {
	parts := []string{access.Mode, strconv.Itoa(access.SelectedCount) + " selected"}
	if len(access.SelectedProjectIDs) > 0 {
		values := make([]string, 0, len(access.SelectedProjectIDs))
		for _, id := range access.SelectedProjectIDs {
			values = append(values, strconv.FormatInt(id, 10))
		}
		parts = append(parts, strings.Join(values, ", "))
	}
	return redact.String(strings.Join(parts, "; "))
}

func spendingSummary(spending api.SpendingAccess) string {
	parts := []string{spending.Mode, "active reserved " + strconv.FormatInt(spending.ActiveReservedCredits, 10)}
	if spending.Allowance != nil {
		parts = append(parts,
			"remaining "+strconv.FormatInt(spending.Allowance.RemainingCredits, 10),
			"limit "+strconv.FormatInt(spending.Allowance.LimitCredits, 10),
		)
	}
	return redact.String(strings.Join(parts, "; "))
}

func ipRestrictionSummary(restrictions api.IPRestrictions) string {
	return fmt.Sprintf("restricted: %t, allow rules: %d, deny rules: %d", restrictions.Restricted, restrictions.AllowRuleCount, restrictions.DenyRuleCount)
}

// capabilitySummary keeps the human output compact while preserving read/write
// ability, scope, and plan-availability signals.
func capabilitySummary(caps []auth.Capability) string {
	if len(caps) == 0 {
		return "none reported"
	}
	parts := make([]string, 0, len(caps))
	for _, cap := range caps {
		abilities := make([]string, 0, 2)
		if cap.Read {
			abilities = append(abilities, "read")
		}
		if cap.Write {
			abilities = append(abilities, "write")
		}
		if len(abilities) == 0 {
			abilities = append(abilities, "no access")
		}
		detail := strings.Join(abilities, "/") + " " + cap.ScopeType
		if !cap.PlanAvailable {
			detail += ", plan unavailable"
		}
		label := cap.ID
		if cap.Label != "" {
			label = cap.Label
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", label, detail))
	}
	return redact.String(strings.Join(parts, "; "))
}

// projectScopeSummary highlights selected-project keys and capped samples
// because callers must not treat the sample as a full project list.
func projectScopeSummary(scope auth.ProjectScope) string {
	switch {
	case scope.SelectedProjects == nil:
		return redact.String(scope.Mode)
	default:
		parts := []string{scope.Mode, strconv.Itoa(scope.SelectedCount) + " selected"}
		if scope.AppliesToTeamLevelGroups {
			parts = append(parts, "applies to team-level groups")
		}
		if len(scope.SelectedProjects.Data) > 0 {
			names := make([]string, 0, len(scope.SelectedProjects.Data))
			for _, project := range scope.SelectedProjects.Data {
				names = append(names, project.Name+" ("+project.ID+")")
			}
			parts = append(parts, strings.Join(names, ", "))
		}
		if scope.SelectedProjects.HasMore {
			parts = append(parts, "whoami returned a capped project summary, not the complete accessible project set")
		}
		return redact.String(strings.Join(parts, "; "))
	}
}

// keyState normalizes API expiration states for human output. Unknown states
// still show an expiration timestamp when the API supplied one.
func keyState(state string, expiresAt *time.Time) string {
	switch state {
	case "expired":
		return "expired"
	case "no_expiration":
		return "never expires"
	case "expiring_soon":
		if expiresAt != nil {
			return "expiring soon (expires at " + formatTime(*expiresAt) + ")"
		}
		return "expiring soon"
	case "active":
		if expiresAt != nil {
			return "expires at " + formatTime(*expiresAt)
		}
		return "active"
	default:
		if expiresAt == nil {
			return "never expires"
		}
		return "expires at " + formatTime(*expiresAt)
	}
}

func formatTime(t time.Time) string {
	return t.Truncate(time.Second).Format(time.RFC3339)
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	value := formatTime(*t)
	return &value
}
