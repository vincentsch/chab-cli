package authcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
)

const (
	minUserCodeLength      = 4
	maxUserCodeLength      = 32
	maxDeviceExpiresIn     = 86400
	maxPollingInterval     = 60
	maxDeviceStartAttempts = 3
	slowDownIncrement      = 5
)

type loginMode int

const (
	loginModeAuto loginMode = iota
	loginModeBrowser
	loginModeManual
)

func loginModeFromFlags(cmd *cobra.Command) (loginMode, error) {
	web, _ := cmd.Flags().GetBool("web")
	apiKey, _ := cmd.Flags().GetBool("api-key")
	switch {
	case web && apiKey:
		return loginModeAuto, &UsageError{Operation: "login", Detail: "--web and --api-key cannot be used together"}
	case web:
		return loginModeBrowser, nil
	case apiKey:
		return loginModeManual, nil
	default:
		return loginModeAuto, nil
	}
}

func runBrowserLogin(
	cmd *cobra.Command,
	f *cmdutil.Factory,
	rt config.Runtime,
	prompter *cmdutil.Prompt,
	shadow environmentShadow,
	bootstrap cmdutil.BootstrapClient,
	clientVersion string,
	scopes []string,
	deviceName string,
) error {
	authFindingsWarned, err := confirmBrowserLoginOverwrite(f, rt, prompter, cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	device, meta, err := createDeviceWithRetry(cmd.Context(), f, bootstrap, clientVersion, scopes, deviceName)
	bootstrap.RegisterSecret(device.DeviceCode)
	f.RegisterSecret(device.DeviceCode)
	if err != nil {
		if scopeErr := invalidScopeError(err, scopes); scopeErr != nil {
			return scopeErr
		}
		return cliAuthEndpointError(err, "device")
	}
	createdAt := f.Clock()()
	if err := validateDevice(device, meta); err != nil {
		return err
	}
	if err := writeBrowserVerification(cmd.ErrOrStderr(), device); err != nil {
		return err
	}
	if prompter == nil || !prompter.NoPrompt() {
		if err := f.OpenBrowser(cmd.Context(), browserOpenURL(device)); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not open browser automatically: %s\n", semanticString(f, err.Error()))
		}
	}

	token, tokenMeta, err := pollBrowserToken(cmd.Context(), f, bootstrap, device, createdAt)
	if err != nil {
		return err
	}
	client, err := authAPIClient(f, rt, auth.Credential{APIKey: token.AccessToken}, cmd, shadow, false)
	if err != nil {
		return err
	}
	data, whoamiMeta, err := client.Whoami(cmd.Context())
	if err != nil {
		return err
	}
	if err := validateBrowserWhoami(data, token, scopes, whoamiMeta, tokenMeta); err != nil {
		return err
	}

	if err := persistValidatedCredential(
		cmd,
		f,
		rt,
		rt.Profile,
		data,
		token.AccessToken,
		authFindingsWarned,
		"login",
		loginConfigPreparation(rt, rt.Profile),
		config.Write,
	); err != nil {
		return err
	}
	presentation := presentationWhoami(f, data)
	profile := semanticString(f, rt.Profile)
	apiBaseURL := semanticString(f, rt.APIBaseURL)
	locale := semanticString(f, rt.Locale)
	if err := f.WriteCommandResult(cmd, cmdutil.CommandResult{
		Human: cmdutil.HumanOutput{Render: func(w io.Writer) {
			renderLoginSuccess(w, profile, apiBaseURL, locale, presentation)
		}, Plain: func(dataW, prose io.Writer) {
			renderLoginPlain(dataW, prose, profile, apiBaseURL, locale, presentation)
		}},
		Supports: cmdutil.OutputSupport{Human: true, Plain: true},
	}); err != nil {
		return err
	}
	warnEnvironmentShadow(cmd, shadow)
	return nil
}

func createDeviceWithRetry(ctx context.Context, f *cmdutil.Factory, bootstrap cmdutil.BootstrapClient, clientVersion string, scopes []string, deviceName string) (api.DeviceData, api.ResponseMeta, error) {
	var lastMeta api.ResponseMeta
	for attempt := 1; attempt <= maxDeviceStartAttempts; attempt++ {
		device, meta, err := bootstrap.CreateDevice(ctx, api.ChabOAuthClientID, clientVersion, scopes, deviceName)
		lastMeta = meta
		if err == nil {
			return device, meta, nil
		}
		if attempt == maxDeviceStartAttempts || !deviceStartRetryable(err) {
			return api.DeviceData{}, meta, err
		}
		wait := deviceStartWait(err)
		if wait == 0 {
			return api.DeviceData{}, meta, err
		}
		if err := f.Sleep(ctx, wait); err != nil {
			return api.DeviceData{}, lastMeta, &api.TransportError{Err: err, Attempts: attempt}
		}
	}
	return api.DeviceData{}, lastMeta, deviceExpiredError()
}

func pollBrowserToken(
	ctx context.Context,
	f *cmdutil.Factory,
	bootstrap cmdutil.BootstrapClient,
	device api.DeviceData,
	createdAt time.Time,
) (api.TokenData, api.ResponseMeta, error) {
	deadline := createdAt.Add(time.Duration(device.ExpiresIn) * time.Second)
	currentInterval := time.Duration(device.Interval) * time.Second
	attempts := 0
	if !pollWaitFitsDeadline(createdAt, currentInterval, deadline) {
		return api.TokenData{}, api.ResponseMeta{}, deviceExpiredError()
	}
	if err := f.Sleep(ctx, currentInterval); err != nil {
		return api.TokenData{}, api.ResponseMeta{}, &api.TransportError{Err: err, Attempts: attempts}
	}

	for {
		now := f.Clock()()
		if !now.Before(deadline) {
			return api.TokenData{}, api.ResponseMeta{}, deviceExpiredError()
		}
		token, meta, err := bootstrap.PollToken(ctx, device.DeviceCode)
		attempts++
		bootstrap.RegisterSecret(token.AccessToken)
		f.RegisterSecret(token.AccessToken)
		if err == nil {
			if err := validateToken(token, meta); err != nil {
				return api.TokenData{}, api.ResponseMeta{}, err
			}
			return token, meta, nil
		}
		var transportErr *api.TransportError
		if errors.As(err, &transportErr) {
			return api.TokenData{}, api.ResponseMeta{}, err
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return api.TokenData{}, api.ResponseMeta{}, &api.TransportError{Err: err, Attempts: attempts}
		}

		wait, retry, terminalErr := nextTokenPollWait(err, currentInterval, f.Clock()(), deadline)
		if terminalErr != nil {
			return api.TokenData{}, api.ResponseMeta{}, terminalErr
		}
		if !retry {
			return api.TokenData{}, api.ResponseMeta{}, err
		}
		currentInterval = wait
		if err := f.Sleep(ctx, wait); err != nil {
			return api.TokenData{}, api.ResponseMeta{}, &api.TransportError{Err: err, Attempts: attempts}
		}
	}
}

func nextTokenPollWait(err error, currentInterval time.Duration, now, deadline time.Time) (time.Duration, bool, error) {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return 0, false, nil
	}

	switch apiErr.Code {
	case "authorization_pending", "temporarily_unavailable":
		wait := currentInterval
		if apiErr.Code == "temporarily_unavailable" {
			if retryAfter := retryAfterPollWait(apiErr, now, deadline); retryAfter > 0 {
				wait = maxDuration(wait, retryAfter)
			}
		}
		if !pollWaitFitsDeadline(now, wait, deadline) {
			return 0, false, deviceExpiredError()
		}
		return wait, true, nil
	case "slow_down":
		wait := slowDownWait(apiErr, currentInterval, now, deadline)
		if wait == 0 {
			return 0, false, deviceExpiredError()
		}
		return wait, true, nil
	case "not_found":
		return 0, false, cliAuthEndpointError(err, "token")
	default:
		return 0, false, nil
	}
}

func deviceStartRetryable(err error) bool {
	var apiErr *api.Error
	return errors.As(err, &apiErr) && apiErr.Code == "temporarily_unavailable"
}

func deviceStartWait(err error) time.Duration {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Meta.RetryAfter.Wait == nil {
		return 0
	}
	if wait := *apiErr.Meta.RetryAfter.Wait; validPollWait(wait) {
		return wait
	}
	return 0
}

func retryAfterPollWait(apiErr *api.Error, now, deadline time.Time) time.Duration {
	if apiErr.Meta.RetryAfter.Wait != nil && validPollWait(*apiErr.Meta.RetryAfter.Wait) && pollWaitFitsDeadline(now, *apiErr.Meta.RetryAfter.Wait, deadline) {
		return *apiErr.Meta.RetryAfter.Wait
	}
	return 0
}

func slowDownWait(apiErr *api.Error, currentInterval time.Duration, now, deadline time.Time) time.Duration {
	if apiErr.Meta.RetryAfter.Wait != nil && validPollWait(*apiErr.Meta.RetryAfter.Wait) && pollWaitFitsDeadline(now, *apiErr.Meta.RetryAfter.Wait, deadline) {
		return maxDuration(currentInterval, *apiErr.Meta.RetryAfter.Wait)
	}
	if interval, ok := rawInterval(apiErr.RawDetails); ok {
		wait := time.Duration(interval) * time.Second
		if validPollWait(wait) && pollWaitFitsDeadline(now, wait, deadline) {
			return wait
		}
	}
	wait := currentInterval + slowDownIncrement*time.Second
	if wait > maxPollingInterval*time.Second {
		wait = maxPollingInterval * time.Second
	}
	if !pollWaitFitsDeadline(now, wait, deadline) {
		return 0
	}
	return wait
}

func maxDuration(a, b time.Duration) time.Duration {
	if b > a {
		return b
	}
	return a
}

func rawInterval(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var details struct {
		Interval *int `json:"interval"`
	}
	if err := json.Unmarshal(raw, &details); err != nil || details.Interval == nil {
		return 0, false
	}
	return *details.Interval, true
}

func pollWaitFitsDeadline(now time.Time, wait time.Duration, deadline time.Time) bool {
	return wait > 0 && now.Add(wait).Before(deadline)
}

func validPollWait(wait time.Duration) bool {
	return wait >= time.Second && wait <= maxPollingInterval*time.Second && wait%time.Second == 0
}

func validateDevice(data api.DeviceData, meta api.ResponseMeta) error {
	if data.DeviceCode == "" {
		return bootstrapProtocol(meta, "device response missing device_code")
	}
	if data.UserCode == "" {
		return bootstrapProtocol(meta, "device response missing user_code")
	}
	if n := utf8.RuneCountInString(data.UserCode); n < minUserCodeLength || n > maxUserCodeLength {
		return bootstrapProtocol(meta, "device response user_code length is outside supported range")
	}
	if strings.IndexFunc(data.UserCode, unicode.IsControl) >= 0 {
		return bootstrapProtocol(meta, "device response user_code must not include control characters")
	}
	if data.ExpiresIn < 1 || data.ExpiresIn > maxDeviceExpiresIn {
		return bootstrapProtocol(meta, "device response expires_in is outside supported range")
	}
	if data.Interval < 1 || data.Interval > maxPollingInterval {
		return bootstrapProtocol(meta, "device response interval is outside supported range")
	}
	if err := validateVerificationURL(data.VerificationURI, false); err != nil {
		return bootstrapProtocolErr(meta, "device response verification_uri is invalid", err)
	}
	if data.VerificationURIComplete != "" {
		if err := validateVerificationURL(data.VerificationURIComplete, true); err != nil {
			return bootstrapProtocolErr(meta, "device response verification_uri_complete is invalid", err)
		}
	}
	return nil
}

func validateToken(data api.TokenData, meta api.ResponseMeta) error {
	if data.AccessToken == "" {
		return bootstrapProtocol(meta, "token response missing access_token")
	}
	if strings.IndexFunc(data.AccessToken, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0 {
		return bootstrapProtocol(meta, "token response access_token must be a single line without whitespace or control characters")
	}
	if data.TokenType != "Bearer" {
		return bootstrapProtocol(meta, "token response token_type is not supported")
	}
	if strings.TrimSpace(data.Scope) == "" {
		return bootstrapProtocol(meta, "token response missing scope")
	}
	if data.ExpiresIn == nil || *data.ExpiresIn < 1 {
		return bootstrapProtocol(meta, "token response expires_in is missing or outside supported range")
	}
	return nil
}

func validateBrowserWhoami(data api.WhoamiData, token api.TokenData, requestedScopes []string, whoamiMeta, tokenMeta api.ResponseMeta) error {
	if err := validateWhoamiIdentity(data, whoamiMeta); err != nil {
		return err
	}
	tokenScopes := strings.Fields(token.Scope)
	if len(tokenScopes) == 0 {
		return bootstrapProtocol(tokenMeta, "token response missing scope")
	}
	if !containsAllScopes(requestedScopes, tokenScopes) {
		return bootstrapProtocol(tokenMeta, "token response scope includes unrequested scopes")
	}
	if !containsAllScopes(data.Scopes, tokenScopes) {
		return bootstrapProtocol(whoamiMeta, "whoami response scopes do not include the issued token scope")
	}
	return nil
}

func validateWhoamiIdentity(data api.WhoamiData, meta api.ResponseMeta) error {
	if data.PrincipalType == "" {
		return bootstrapProtocol(meta, "whoami response missing principal_type")
	}
	if data.PrincipalType == "guest_trial" {
		if data.PrincipalID == "" || data.GuestID == "" {
			return bootstrapProtocol(meta, "guest whoami response missing principal_id or guest_id")
		}
		return nil
	}
	if data.TeamID == 0 {
		return bootstrapProtocol(meta, "whoami response missing team_id")
	}
	if data.TokenPublicID == "" {
		return bootstrapProtocol(meta, "whoami response missing token_public_id")
	}
	return nil
}

func containsAllScopes(haystack, needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	seen := make(map[string]bool, len(haystack))
	for _, scope := range haystack {
		if scope != "" {
			seen[scope] = true
		}
	}
	for _, scope := range needles {
		if scope != "" && !seen[scope] {
			return false
		}
	}
	return true
}

func confirmBrowserLoginOverwrite(f *cmdutil.Factory, rt config.Runtime, prompter *cmdutil.Prompt, stderr io.Writer) (bool, error) {
	if prompter != nil && prompter.Yes() {
		return false, nil
	}
	file, findings, err := auth.Load(rt.AuthPath)
	authFindingsWarned := len(findings) > 0
	var record auth.ProfileAuth
	var recordExists bool
	if err == nil {
		record, recordExists = file.Profiles[rt.Profile]
		if recordExists && record.APIKey != "" {
			f.RegisterSecret(record.APIKey)
		}
	}
	cmdutil.WarnPermissionFindings(stderr, presentationPermissionFindings(f, findings))
	if err != nil {
		return authFindingsWarned, err
	}
	if !recordExists || record.APIKey == "" {
		return authFindingsWarned, nil
	}
	if prompter == nil || prompter.NoPrompt() {
		return authFindingsWarned, &cmdutil.AbortError{Message: fmt.Sprintf("stored credential already exists for profile %q; re-run with --yes to overwrite without a prompt", semanticString(f, rt.Profile))}
	}
	if !prompter.Interactive() {
		return authFindingsWarned, &cmdutil.AbortError{Message: "this action needs confirmation; re-run with --yes to proceed without a prompt"}
	}
	confirmed, err := prompter.Confirm(loginOverwriteQuestion(f, rt.Profile, record))
	if err != nil {
		return authFindingsWarned, err
	}
	if !confirmed {
		return authFindingsWarned, &cmdutil.AbortError{Message: "canceled; no changes were made"}
	}
	return authFindingsWarned, nil
}

func requireStringPtr(meta api.ResponseMeta, field string, value *string, want string) error {
	if value == nil || *value == "" {
		return bootstrapProtocol(meta, "discovery response missing "+field)
	}
	if *value != want {
		return bootstrapProtocol(meta, "discovery response "+field+" is not supported")
	}
	return nil
}

func requireIntRangePtr(meta api.ResponseMeta, field string, value *int, minValue, maxValue int) error {
	if value == nil {
		return bootstrapProtocol(meta, "discovery response missing "+field)
	}
	if *value < minValue || *value > maxValue {
		return bootstrapProtocol(meta, "discovery response "+field+" is outside supported range")
	}
	return nil
}

func requireEndpointPtr(meta api.ResponseMeta, field string, value *string, expected string) error {
	if value == nil || *value == "" {
		return bootstrapProtocol(meta, "discovery response missing "+field)
	}
	got, err := api.NormalizeURLForMatch(*value)
	if err != nil {
		return bootstrapProtocolErr(meta, "discovery response "+field+" is invalid", err)
	}
	want, err := api.NormalizeURLForMatch(expected)
	if err != nil {
		return bootstrapProtocolErr(meta, "resolved "+field+" is invalid", err)
	}
	if got != want {
		return bootstrapProtocol(meta, "discovery response "+field+" does not match the resolved API base URL")
	}
	return nil
}

func validateVerificationURL(raw string, allowQuery bool) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("URL must not be empty")
	}
	if strings.TrimSpace(raw) != raw {
		return fmt.Errorf("URL must not include leading or trailing whitespace")
	}
	if strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return fmt.Errorf("URL must not include control characters")
	}
	if strings.Contains(raw, "#") {
		return fmt.Errorf("fragment is not allowed")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if !parsed.IsAbs() {
		return fmt.Errorf("URL must be absolute")
	}
	if parsed.Host == "" {
		return fmt.Errorf("missing host")
	}
	if parsed.User != nil {
		return fmt.Errorf("userinfo is not allowed")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("fragment is not allowed")
	}
	if !allowQuery && (parsed.RawQuery != "" || parsed.ForceQuery) {
		return fmt.Errorf("query is not allowed")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && allowedLoopbackHost(parsed.Hostname()) {
		return nil
	}
	return fmt.Errorf("scheme must be https unless the host is loopback localhost")
}

func allowedLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func writeBrowserVerification(w io.Writer, device api.DeviceData) error {
	verificationURI := output.SanitizeInlineText(device.VerificationURI)
	userCode := output.SanitizeInlineText(device.UserCode)
	_, err := fmt.Fprintf(w, "Complete browser login:\nVerification URL: %s\nUser code: %s\n", verificationURI, userCode)
	return err
}

func browserOpenURL(device api.DeviceData) string {
	if device.VerificationURIComplete != "" {
		return device.VerificationURIComplete
	}
	return device.VerificationURI
}

func browserLoginUnavailableFromDiscovery(err error) bool {
	var apiErr *api.Error
	return errors.As(err, &apiErr) && apiErr.Code == "not_found"
}

func browserLoginUnavailable(reason string) error {
	return &UsageError{Operation: "login", Detail: "browser login is unavailable: " + reason + "; re-run with --api-key for manual entry"}
}

func cliAuthEndpointError(err error, endpoint string) error {
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Code == "not_found" {
		return &api.ProtocolError{
			Detail:    "OAuth " + endpoint + " endpoint returned not_found",
			Status:    apiErr.Status,
			RequestID: apiErr.RequestID,
			Err:       err,
			Meta:      apiErr.Meta,
		}
	}
	return err
}

func invalidScopeError(err error, scopes []string) error {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "invalid_scope" {
		return nil
	}
	requested := strings.Join(scopes, " ")
	docs := firstDetailString(apiErr.DetailsValue, "docs_url", "documentation_url", "documentation")
	return &invalidScopeContext{err: err, requested: requested, docs: docs}
}

type invalidScopeContext struct {
	err       error
	requested string
	docs      string
}

func (e *invalidScopeContext) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(e.err.Error())
	if e.requested != "" {
		b.WriteString("; requested scopes: ")
		b.WriteString(e.requested)
	}
	if e.docs != "" {
		b.WriteString("; docs: ")
		b.WriteString(e.docs)
	}
	return b.String()
}

func (e *invalidScopeContext) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *invalidScopeContext) APIErrorNotes() []string {
	if e == nil {
		return nil
	}
	var notes []string
	if e.requested != "" {
		notes = append(notes, "Requested scopes: "+e.requested)
	}
	if e.docs != "" {
		notes = append(notes, "Documentation: "+e.docs)
	}
	return notes
}

func firstDetailString(value any, keys ...string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if raw, ok := typed[key].(string); ok && raw != "" {
				return raw
			}
		}
		for _, nested := range typed {
			if found := firstDetailString(nested, keys...); found != "" {
				return found
			}
		}
	case []any:
		for _, nested := range typed {
			if found := firstDetailString(nested, keys...); found != "" {
				return found
			}
		}
	}
	return ""
}

func deviceExpiredError() error {
	return &api.Error{Code: "expired_token", Message: "Device authorization expired."}
}

func bootstrapProtocol(meta api.ResponseMeta, detail string) error {
	return bootstrapProtocolErr(meta, detail, nil)
}

func bootstrapProtocolErr(meta api.ResponseMeta, detail string, err error) error {
	return &api.ProtocolError{
		Detail:    detail,
		Status:    meta.HTTPStatus,
		RequestID: meta.RequestID,
		Err:       err,
		Meta:      meta,
	}
}
