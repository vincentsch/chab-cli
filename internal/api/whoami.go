package api

import (
	"context"
	"encoding/json"
)

// WhoamiData is the decoded data payload of GET /me.
type WhoamiData struct {
	PrincipalType string          `json:"principal_type"`
	PrincipalID   string          `json:"principal_id,omitempty"`
	GuestID       string          `json:"guest_id,omitempty"`
	Credential    json.RawMessage `json:"credential,omitempty"`
	FreeAccess    json.RawMessage `json:"free_access,omitempty"`
	TeamID        int64           `json:"team_id"`
	TokenID       string          `json:"token_id"`
	TokenPublicID string          `json:"token_public_id"`
	Scopes        []string        `json:"scopes"`
	TokenControls TokenControls   `json:"token_controls"`
	RequestID     string          `json:"request_id"`
}

// MarshalJSON preserves the guest identity's null team and guest-specific
// balance/credential context instead of inventing a team ID of zero.
func (d WhoamiData) MarshalJSON() ([]byte, error) {
	if d.PrincipalType != "guest_trial" {
		type wire WhoamiData
		return json.Marshal(wire(d))
	}
	return json.Marshal(struct {
		PrincipalType string          `json:"principal_type"`
		TeamID        *int64          `json:"team_id"`
		GuestID       string          `json:"guest_id"`
		PrincipalID   string          `json:"principal_id"`
		Scopes        []string        `json:"scopes"`
		Credential    json.RawMessage `json:"credential,omitempty"`
		FreeAccess    json.RawMessage `json:"free_access,omitempty"`
		RequestID     string          `json:"request_id"`
	}{d.PrincipalType, nil, d.GuestID, d.PrincipalID, d.Scopes, d.Credential, d.FreeAccess, d.RequestID})
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

// Whoami fetches the authenticated Chab token context.
func (c *Client) Whoami(ctx context.Context) (WhoamiData, ResponseMeta, error) {
	var data WhoamiData
	meta, err := c.Get(ctx, "me", nil, &data)
	return data, meta, err
}
