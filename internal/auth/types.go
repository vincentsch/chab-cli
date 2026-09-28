package auth

import (
	"encoding/json"
	"os"
	"time"
)

const fileVersion = 1

// File is the known-field view of auth.json. Unknown JSON object keys are kept
// internally after Load and preserved by Write.
type File struct {
	Version  int
	Profiles map[string]ProfileAuth

	raw        map[string]json.RawMessage
	profileRaw map[string]map[string]json.RawMessage
	exists     bool
}

// Exists reports whether Load read an existing auth file. Missing files
// produce a valid empty in-memory file and return false here.
func (f *File) Exists() bool {
	return f != nil && f.exists
}

// ProfileAuth stores the secret API key plus cached non-secret whoami metadata.
type ProfileAuth struct {
	APIKey             string         `json:"api_key"`
	PrincipalType      string         `json:"principal_type,omitempty"`
	PrincipalID        string         `json:"principal_id,omitempty"`
	TeamID             int64          `json:"team_id,omitempty"`
	TokenID            string         `json:"token_id,omitempty"`
	TokenPublicID      string         `json:"token_public_id,omitempty"`
	Scopes             []string       `json:"scopes,omitempty"`
	TokenControls      *TokenControls `json:"token_controls,omitempty"`
	DisplayID          string         `json:"display_id,omitempty"`
	KeyName            string         `json:"key_name,omitempty"`
	KeyPreset          string         `json:"key_preset,omitempty"`
	KeyExpirationState string         `json:"key_expiration_state,omitempty"`
	KeyLastUsedAt      *time.Time     `json:"key_last_used_at,omitempty"`
	KeyCreatedAt       *time.Time     `json:"key_created_at,omitempty"`
	TeamDisplayID      string         `json:"team_display_id,omitempty"`
	TeamName           string         `json:"team_name,omitempty"`
	Plan               *Plan          `json:"plan,omitempty"`
	Capabilities       []Capability   `json:"capabilities,omitempty"`
	ProjectScope       *ProjectScope  `json:"project_scope,omitempty"`
	ExpiresAt          *time.Time     `json:"expires_at,omitempty"`
	LastValidatedAt    *time.Time     `json:"last_validated_at,omitempty"`
}

type TokenControls struct {
	PolicyRevision int            `json:"policy_revision"`
	FeatureAccess  FeatureAccess  `json:"feature_access"`
	IPRestrictions IPRestrictions `json:"ip_restrictions"`
	Spending       SpendingAccess `json:"spending"`
	ProjectAccess  ProjectAccess  `json:"project_access"`
}

type FeatureAccess struct {
	Mode          string `json:"mode"`
	SnapshotStale bool   `json:"snapshot_stale"`
}

type IPRestrictions struct {
	Restricted     bool `json:"restricted"`
	AllowRuleCount int  `json:"allow_rule_count"`
	DenyRuleCount  int  `json:"deny_rule_count"`
}

type SpendingAccess struct {
	Mode                  string          `json:"mode"`
	ActiveReservedCredits int64           `json:"active_reserved_credits"`
	Allowance             *TokenAllowance `json:"allowance"`
}

type TokenAllowance struct {
	Version          int   `json:"version"`
	LimitCredits     int64 `json:"limit_credits"`
	SettledCredits   int64 `json:"settled_credits"`
	HeldCredits      int64 `json:"held_credits"`
	RemainingCredits int64 `json:"remaining_credits"`
}

type ProjectAccess struct {
	Mode               string  `json:"mode"`
	SelectedProjectIDs []int64 `json:"selected_project_ids"`
	SelectedCount      int     `json:"selected_count"`
}

// Plan is the cached subscription summary (nested even though the wire is flat).
type Plan struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	APIAccess bool   `json:"api_access"`
}

// Capability is one flat API capability row from whoami.
type Capability struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Description   string `json:"description"`
	Read          bool   `json:"read"`
	Write         bool   `json:"write"`
	ScopeType     string `json:"scope_type"`
	PlanAvailable bool   `json:"plan_available"`
}

// ProjectScope records all-projects vs selected-project access. SelectedCount
// and AppliesToTeamLevelGroups are scope-level fields.
type ProjectScope struct {
	Mode                     string            `json:"mode"`
	SelectedCount            int               `json:"selected_count"`
	SelectedProjects         *SelectedProjects `json:"selected_projects,omitempty"`
	AppliesToTeamLevelGroups bool              `json:"applies_to_team_level_groups"`
}

// SelectedProjects is the capped selected-project sample for project-scoped keys.
type SelectedProjects struct {
	Data    []ProjectSummary `json:"data"`
	HasMore bool             `json:"has_more"`
}

// ProjectSummary is the small project shape embedded in auth metadata.
type ProjectSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PermissionFinding reports an auth file mode that is broader than expected.
// It is a warning for callers to display, not a reason Load must fail.
type PermissionFinding struct {
	Path         string
	ActualMode   os.FileMode
	ExpectedMode os.FileMode
}

// CredentialSource identifies where the selected API key came from.
type CredentialSource string

const (
	SourceEnv      CredentialSource = "env"
	SourceAuthFile CredentialSource = "auth_file"
)

// Options injects process-dependent inputs for credential lookup.
type Options struct {
	LookupEnv func(string) (string, bool)
}

// Credential is the selected API key plus non-secret display metadata. Its
// string forms intentionally omit APIKey so formatting cannot leak secrets.
type Credential struct {
	APIKey        string
	Source        CredentialSource
	Profile       string
	DisplayID     string
	KeyName       string
	PrincipalType string
}
