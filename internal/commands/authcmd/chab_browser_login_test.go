package authcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/output"
)

func TestChabLoginScopesFromFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "login"}
	cmd.Flags().StringArray("scope", nil, "")
	if err := cmd.Flags().Set("scope", "api:projects:read api:credits:read"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("scope", "api:credits:read"); err != nil {
		t.Fatal(err)
	}
	got, err := loginScopesFromFlags(cmd)
	if err != nil {
		t.Fatalf("loginScopesFromFlags: %v", err)
	}
	want := "api:projects:read api:credits:read"
	if strings.Join(got, " ") != want {
		t.Fatalf("scopes = %q, want %q", strings.Join(got, " "), want)
	}
}

func TestGuestIdentityCanBeStoredWithoutTeamToken(t *testing.T) {
	data := api.WhoamiData{PrincipalType: "guest_trial", PrincipalID: "guest_trial:gtp_one", GuestID: "gt_one", Scopes: []string{"api:credits:read"}}
	if err := validateWhoamiIdentity(data, api.ResponseMeta{}); err != nil {
		t.Fatalf("guest identity rejected: %v", err)
	}
	record := profileAuthFromWhoami(data, "chab_guest_dummy", time.Now())
	if record.PrincipalID != data.PrincipalID || record.DisplayID != data.PrincipalID || record.TokenPublicID != "" || record.TeamID != 0 {
		t.Fatalf("guest auth record lost principal separation: %#v", record)
	}
	var plain bytes.Buffer
	renderLoginPlain(&plain, &plain, "local", "https://example.test", "en", data)
	if !strings.Contains(plain.String(), "principal_id\tguest_trial:gtp_one") || strings.Contains(plain.String(), "team_id\t0") || strings.Contains(plain.String(), "token_public_id\t") {
		t.Fatalf("guest plain login identity = %s", plain.String())
	}
}

func TestChabBrowserLoginValidatesCurrentShapes(t *testing.T) {
	meta := api.ResponseMeta{HTTPStatus: 200, RequestID: "req"}
	device := api.DeviceData{
		DeviceCode:              "secret-device",
		UserCode:                "ABCD-EFGH",
		VerificationURI:         "https://example.test/verify",
		VerificationURIComplete: "https://example.test/verify?user_code=ABCD-EFGH",
		ExpiresIn:               600,
		Interval:                2,
	}
	if err := validateDevice(device, meta); err != nil {
		t.Fatalf("validateDevice: %v", err)
	}
	expiresIn := 600
	if err := validateToken(api.TokenData{AccessToken: "secret-token", TokenType: "Bearer", Scope: "api:projects:read", ExpiresIn: &expiresIn}, meta); err != nil {
		t.Fatalf("validateToken: %v", err)
	}
	whoami := api.WhoamiData{
		PrincipalType: "team_token",
		TeamID:        42,
		TokenPublicID: "tok_123",
		TokenID:       "1",
		Scopes:        []string{"api:projects:read"},
	}
	if err := validateBrowserWhoami(whoami, api.TokenData{Scope: "api:projects:read", ExpiresIn: &expiresIn}, []string{"api:projects:read"}, meta, meta); err != nil {
		t.Fatalf("validateBrowserWhoami: %v", err)
	}
}

func TestChabBrowserLoginRejectsIncompleteTokenAndUnexpectedScopes(t *testing.T) {
	meta := api.ResponseMeta{HTTPStatus: 200, RequestID: "req"}
	if err := validateToken(api.TokenData{AccessToken: "secret-token", TokenType: "Bearer", Scope: "api:credits:read"}, meta); err == nil || !strings.Contains(err.Error(), "expires_in") {
		t.Fatalf("validateToken() error = %v, want expires_in failure", err)
	}

	expiresIn := 600
	whoami := api.WhoamiData{
		PrincipalType: "team",
		TeamID:        42,
		TokenPublicID: "tok_public",
		TokenID:       "tok_internal",
		Scopes:        []string{"api:credits:read"},
	}
	token := api.TokenData{AccessToken: "secret-token", TokenType: "Bearer", Scope: "api:credits:read", ExpiresIn: &expiresIn}
	if err := validateBrowserWhoami(whoami, token, []string{"api:projects:read", "api:credits:read"}, meta, meta); err != nil {
		t.Fatalf("validateBrowserWhoami(narrowed consent) error = %v", err)
	}
	if err := validateBrowserWhoami(whoami, token, []string{"api:projects:read"}, meta, meta); err == nil || !strings.Contains(err.Error(), "unrequested scopes") {
		t.Fatalf("validateBrowserWhoami(unrequested scope) error = %v", err)
	}
	whoami.Scopes = []string{"api:projects:read"}
	if err := validateBrowserWhoami(whoami, token, []string{"api:credits:read"}, meta, meta); err == nil || !strings.Contains(err.Error(), "issued token scope") {
		t.Fatalf("validateBrowserWhoami(whoami mismatch) error = %v", err)
	}
}

func TestChabOAuthRetryAfterControlsDeviceStartAndPollWaits(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	wait := 3 * time.Second
	apiErr := &api.Error{
		Code:      "temporarily_unavailable",
		Message:   "try later",
		Retryable: true,
		Meta:      retryAfterMeta(wait),
	}
	got, retry, terminal := nextTokenPollWait(apiErr, time.Second, now, now.Add(time.Minute))
	if terminal != nil || !retry || got != wait {
		t.Fatalf("nextTokenPollWait() = wait:%s retry:%t terminal:%v", got, retry, terminal)
	}
	shortWait := time.Second
	shortRetryAfter := &api.Error{
		Code:      "temporarily_unavailable",
		Message:   "try later",
		Retryable: true,
		Meta:      retryAfterMeta(shortWait),
	}
	got, retry, terminal = nextTokenPollWait(shortRetryAfter, 5*time.Second, now, now.Add(time.Minute))
	if terminal != nil || !retry || got != 5*time.Second {
		t.Fatalf("short Retry-After nextTokenPollWait() = wait:%s retry:%t terminal:%v", got, retry, terminal)
	}

	sleeper := &recordingSleeper{}
	bootstrap := &retryingBootstrap{
		errs: []error{apiErr},
		device: api.DeviceData{
			DeviceCode:      "device-secret",
			UserCode:        "ABCD-EFGH",
			VerificationURI: "https://example.test/verify",
			ExpiresIn:       600,
			Interval:        1,
		},
	}
	device, _, err := createDeviceWithRetry(context.Background(), &cmdutil.Factory{Sleeper: sleeper}, bootstrap, "1.2.3", []string{"api:credits:read"}, "workstation")
	if err != nil {
		t.Fatalf("createDeviceWithRetry() error = %v", err)
	}
	if device.DeviceCode != "device-secret" || len(sleeper.waits) != 1 || sleeper.waits[0] != wait {
		t.Fatalf("device=%#v waits=%v", device, sleeper.waits)
	}
}

func TestChabOAuthPollWaitsBeforeFirstRequest(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	expiresIn := 600
	sleeper := &recordingSleeper{}
	bootstrap := &retryingBootstrap{
		token: api.TokenData{
			AccessToken: "secret-token",
			TokenType:   "Bearer",
			Scope:       "api:projects:read",
			ExpiresIn:   &expiresIn,
		},
	}
	device := api.DeviceData{
		DeviceCode: "device-secret",
		ExpiresIn:  600,
		Interval:   5,
	}

	token, _, err := pollBrowserToken(context.Background(), &cmdutil.Factory{
		Sleeper: sleeper,
		Now:     func() time.Time { return now },
	}, bootstrap, device, now)
	if err != nil {
		t.Fatalf("pollBrowserToken() error = %v", err)
	}
	if token.AccessToken != "secret-token" || bootstrap.pollCalls != 1 {
		t.Fatalf("token=%#v pollCalls=%d", token, bootstrap.pollCalls)
	}
	if len(sleeper.waits) != 1 || sleeper.waits[0] != 5*time.Second {
		t.Fatalf("poll waits = %v, want initial 5s wait", sleeper.waits)
	}
}

func TestInvalidScopeErrorAddsRequestedScopesAndDocumentation(t *testing.T) {
	err := invalidScopeError(&api.Error{
		Code:         "invalid_scope",
		Message:      "Scopes are invalid.",
		Retryable:    false,
		DetailsValue: map[string]any{"documentation_url": "https://docs.example.test/scopes"},
	}, []string{"api:credits:read", "api:projects:read"})
	if err == nil {
		t.Fatal("invalidScopeError() = nil")
	}
	var buf bytes.Buffer
	if !output.WriteAPIErrorHuman(&buf, err) {
		t.Fatal("WriteAPIErrorHuman() = false")
	}
	text := buf.String()
	for _, want := range []string{
		"Requested scopes: api:credits:read api:projects:read",
		"Documentation: https://docs.example.test/scopes",
		"Remediation: Review the request fields and documented constraints.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("human error missing %q:\n%s", want, text)
		}
	}
}

func retryAfterMeta(wait time.Duration) api.ResponseMeta {
	return api.ResponseMeta{RetryAfter: api.RetryAfter{Wait: &wait, Raw: "3", Present: true}}
}

type recordingSleeper struct {
	waits []time.Duration
}

func (s *recordingSleeper) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.waits = append(s.waits, d)
	return nil
}

type retryingBootstrap struct {
	errs      []error
	device    api.DeviceData
	token     api.TokenData
	calls     int
	pollCalls int
}

func (b *retryingBootstrap) RegisterSecret(string)  {}
func (b *retryingBootstrap) DeviceEndpoint() string { return "https://example.test/oauth/device/code" }
func (b *retryingBootstrap) TokenEndpoint() string  { return "https://example.test/oauth/token" }
func (b *retryingBootstrap) Compatibility(context.Context, string) (api.CompatibilityData, api.ResponseMeta, error) {
	return api.CompatibilityData{}, api.ResponseMeta{}, nil
}
func (b *retryingBootstrap) CreateDevice(context.Context, string, string, []string, string) (api.DeviceData, api.ResponseMeta, error) {
	if b.calls < len(b.errs) {
		err := b.errs[b.calls]
		b.calls++
		return api.DeviceData{}, retryAfterMeta(3 * time.Second), err
	}
	b.calls++
	return b.device, api.ResponseMeta{HTTPStatus: 200, RequestID: "req-device"}, nil
}
func (b *retryingBootstrap) PollToken(context.Context, string) (api.TokenData, api.ResponseMeta, error) {
	b.pollCalls++
	return b.token, api.ResponseMeta{HTTPStatus: 200, RequestID: "req-token"}, nil
}
