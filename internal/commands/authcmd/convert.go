package authcmd

import (
	"time"

	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
)

// profileAuthFromWhoami maps the live /me contract into the local non-secret
// auth cache after the caller's API key has been validated.
func profileAuthFromWhoami(data api.WhoamiData, key string, validatedAt time.Time) auth.ProfileAuth {
	return auth.ProfileAuth{
		APIKey:          key,
		PrincipalType:   data.PrincipalType,
		PrincipalID:     data.PrincipalID,
		TeamID:          data.TeamID,
		TokenID:         data.TokenID,
		TokenPublicID:   data.TokenPublicID,
		DisplayID:       displayIdentity(data),
		Scopes:          append([]string(nil), data.Scopes...),
		TokenControls:   authTokenControls(data.TokenControls),
		LastValidatedAt: ptrTime(validatedAt),
	}
}

func displayIdentity(data api.WhoamiData) string {
	if data.PrincipalType == "guest_trial" {
		return data.PrincipalID
	}
	return data.TokenPublicID
}

func authTokenControls(in api.TokenControls) *auth.TokenControls {
	out := auth.TokenControls{
		PolicyRevision: in.PolicyRevision,
		FeatureAccess: auth.FeatureAccess{
			Mode:          in.FeatureAccess.Mode,
			SnapshotStale: in.FeatureAccess.SnapshotStale,
		},
		IPRestrictions: auth.IPRestrictions{
			Restricted:     in.IPRestrictions.Restricted,
			AllowRuleCount: in.IPRestrictions.AllowRuleCount,
			DenyRuleCount:  in.IPRestrictions.DenyRuleCount,
		},
		Spending: auth.SpendingAccess{
			Mode:                  in.Spending.Mode,
			ActiveReservedCredits: in.Spending.ActiveReservedCredits,
		},
		ProjectAccess: auth.ProjectAccess{
			Mode:               in.ProjectAccess.Mode,
			SelectedProjectIDs: append([]int64(nil), in.ProjectAccess.SelectedProjectIDs...),
			SelectedCount:      in.ProjectAccess.SelectedCount,
		},
	}
	if in.Spending.Allowance != nil {
		out.Spending.Allowance = &auth.TokenAllowance{
			Version:          in.Spending.Allowance.Version,
			LimitCredits:     in.Spending.Allowance.LimitCredits,
			SettledCredits:   in.Spending.Allowance.SettledCredits,
			HeldCredits:      in.Spending.Allowance.HeldCredits,
			RemainingCredits: in.Spending.Allowance.RemainingCredits,
		}
	}
	return &out
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
