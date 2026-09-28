package api

import (
	"encoding/json"
	"testing"
)

func TestGuestWhoamiMarshalsNullTeamAndFreeAccess(t *testing.T) {
	value := WhoamiData{
		PrincipalType: "guest_trial",
		PrincipalID:   "guest_trial:gtp_one",
		GuestID:       "gt_one",
		Scopes:        []string{"api:credits:read"},
		Credential:    json.RawMessage(`{"id":"gtc_one","transport":"api_cli"}`),
		FreeAccess:    json.RawMessage(`{"remaining_credits":40}`),
		RequestID:     "req_guest",
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["team_id"] != nil || decoded["principal_id"] != value.PrincipalID || decoded["token_public_id"] != nil {
		t.Fatalf("guest whoami invented team or token identity: %s", data)
	}
	if decoded["credential"].(map[string]any)["id"] != "gtc_one" || decoded["free_access"].(map[string]any)["remaining_credits"] != float64(40) {
		t.Fatalf("guest whoami lost API fields: %s", data)
	}
}
