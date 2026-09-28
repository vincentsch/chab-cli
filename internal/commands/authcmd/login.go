package authcmd

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
)

// UsageError is a local authentication-command input error raised before an
// API call. Operation keeps shared login/setup helpers command-specific.
type UsageError struct {
	Operation string
	Detail    string
}

func (e *UsageError) Error() string {
	operation := e.Operation
	if operation == "" {
		operation = "login"
	}
	return operation + " usage error: " + e.Detail
}

func (e *UsageError) ExitCode() int {
	return 1
}

// NewLoginCommand builds chab auth login.
func NewLoginCommand(f *cmdutil.Factory) *cobra.Command {
	return newLoginCommand(f, "login", loginLong(false), "auth login")
}

// NewLoginAlias builds chab login.
func NewLoginAlias(f *cmdutil.Factory) *cobra.Command {
	cmd := newLoginCommand(f, "login", loginLong(true), "login")
	return cmd
}

func newLoginCommand(f *cmdutil.Factory, use, long, invocation string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: "Authorize and store an API or guest trial key",
		Long:  long,
		Example: fmt.Sprintf(`  chab %s
  chab %s --web
  chab %s --api-key
  chab %s --profile staging --base-url https://example.test
  chab %s --plain --api-key --no-prompt --yes < api-key.txt`, invocation, invocation, invocation, invocation, invocation),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd, f)
		},
	}
	cmd.Flags().Bool("web", false, "use browser device login")
	cmd.Flags().Bool("api-key", false, "use manual API-key entry instead of browser login")
	cmd.Flags().StringArray("scope", nil, "request an OAuth scope; may be repeated or space-separated")
	cmd.Flags().String("device-name", "", "label this browser authorization device")
	cmd.MarkFlagsMutuallyExclusive("web", "api-key")
	return cmd
}

func loginLong(alias bool) string {
	text := `Authorize the CLI with browser device login when available, or validate a copied team API key or guest trial credential with GET /v1/me and store it for future commands.

Team API keys are created and revoked in the product web app. Guest trial credentials are issued by the guest trial flow. Manual API-key
entry reads the key from a secret prompt or from non-terminal stdin; it is
never accepted from a flag or CHAB_API_KEY. --api-key only selects manual
entry and does not accept the key value.

Interactive input prompts for the product base URL only when the built-in
default is still selected. Values supplied by --base-url, CHAB_BASE_URL, or the
selected profile are used as-is.

For browser login against a custom API host, also set the matching app origin
with --base-url or CHAB_BASE_URL. An API-only override cannot use an implicit
browser issuer.

With no selector, interactive login checks CLI compatibility, prints the
verification URL and user code to stderr, tries to open the browser, polls until
approval or terminal failure, validates the issued API key with GET /v1/me, then
stores it. Use --scope to request explicit space-separated or repeated OAuth
scopes; the default minimal scope is api:projects:read.

Use --web to force browser login. With --web --no-prompt, login prints the
verification URL and user code without opening a browser. Use --api-key to
force manual entry and skip discovery. Non-terminal default login also uses
manual entry without discovery, preserving piped-key automation. With
--no-prompt and terminal stdin, manual login fails with guidance to pipe the
key.

The write target is the resolved profile from --profile, CHAB_PROFILE,
current_profile, or the built-in local profile. When that profile already has a
stored key, interactive login asks before reading the replacement key. --yes
accepts that overwrite confirmation, but it does not provide missing input.
With --no-prompt, an overwrite without --yes fails before stdin is consumed,
the browser is opened, or an authenticated API request is made. Non-terminal
manual stdin without --no-prompt keeps the compatible piped-key path and
overwrites after successful API validation.

Output is human-readable even when --json is inherited from the root. Use
--plain for copy-safe login result rows. jq and template output are not
available because login has no stable JSON shape.

If CHAB_API_KEY is set, login still validates and stores the manual or
browser-issued key, then warns after successful output that the environment
credential takes precedence for ordinary authenticated commands.

Related commands:
  chab whoami
  chab auth status
  chab doctor`
	if alias {
		return text + "\n\nThis shorthand mirrors chab auth login."
	}
	return text
}

// runLogin chooses browser or manual authorization after runtime resolution.
// Both paths validate the resulting API key remotely before persisting it.
// Environment-shadow redaction is installed before prompts, and its warning is
// delayed until successful result rendering.
func runLogin(cmd *cobra.Command, f *cmdutil.Factory) error {
	shadow := lookupEnvironmentShadow(f)
	registerEnvironmentShadow(f, shadow)

	mode, err := loginModeFromFlags(cmd)
	if err != nil {
		return err
	}
	flags := cmdutil.RuntimeFlagOverrides(cmd)
	rt, err := f.ResolveRuntimeWithFlags(flags, config.ResolveForWrite)
	if err != nil {
		return err
	}

	prompter := f.Prompt(cmd)
	if prompter.Interactive() {
		rt, flags, err = promptLoginRuntime(f, prompter, flags, rt, "login")
		if err != nil {
			return err
		}
	}

	if mode == loginModeManual || (mode == loginModeAuto && !prompter.Interactive()) {
		return runManualLogin(cmd, f, rt, prompter, shadow)
	}
	// A custom REST host alone must not silently borrow the implicit browser
	// issuer (production or a legacy localhost fallback).
	implicitApp := rt.BaseURLSource == config.RuntimeValueSourceDefault ||
		(rt.BaseURLSource == config.RuntimeValueSourceFile && rt.BaseURL == config.DefaultBaseURL)
	if implicitApp && rt.APIBaseURL != config.DeriveAPIBaseURL(rt.BaseURL) {
		if mode == loginModeAuto {
			return runManualLogin(cmd, f, rt, prompter, shadow)
		}
		return &UsageError{Operation: "login", Detail: "a custom API URL requires --base-url (or CHAB_BASE_URL) for browser login; choose the matching app origin"}
	}

	scopes, err := loginScopesFromFlags(cmd)
	if err != nil {
		return err
	}
	deviceName, _ := cmd.Flags().GetString("device-name")
	clientVersion := api.EffectiveClientVersion(f.VersionString())
	bootstrap, err := f.BootstrapClient(rt, cmd)
	if err != nil {
		return err
	}
	if _, _, err := bootstrap.Compatibility(cmd.Context(), clientVersion); err != nil {
		if browserLoginUnavailableFromDiscovery(err) && mode == loginModeAuto {
			return runManualLogin(cmd, f, rt, prompter, shadow)
		}
		return err
	}
	return runBrowserLogin(cmd, f, rt, prompter, shadow, bootstrap, clientVersion, scopes, deviceName)
}

func loginScopesFromFlags(cmd *cobra.Command) ([]string, error) {
	raw, _ := cmd.Flags().GetStringArray("scope")
	if len(raw) == 0 {
		return []string{"api:projects:read"}, nil
	}
	seen := map[string]struct{}{}
	var scopes []string
	for _, item := range raw {
		for _, scope := range strings.Fields(item) {
			if _, ok := seen[scope]; ok {
				continue
			}
			seen[scope] = struct{}{}
			scopes = append(scopes, scope)
		}
	}
	if len(scopes) == 0 {
		return nil, &UsageError{Operation: "login", Detail: "--scope must include at least one non-empty scope"}
	}
	return scopes, nil
}

func runManualLogin(cmd *cobra.Command, f *cmdutil.Factory, rt config.Runtime, prompter *cmdutil.Prompt, shadow environmentShadow) error {
	authFindingsWarned, err := confirmManualLoginOverwrite(f, rt, prompter, cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	key, err := readLoginKey(prompter, "login")
	if err != nil {
		return err
	}
	f.RegisterSecret(key)

	client, err := authAPIClient(f, rt, auth.Credential{APIKey: key}, cmd, shadow, false)
	if err != nil {
		return err
	}
	data, meta, err := client.Whoami(cmd.Context())
	if err != nil {
		return err
	}
	if err := validateWhoamiIdentity(data, meta); err != nil {
		return err
	}

	if err := persistValidatedCredential(
		cmd,
		f,
		rt,
		rt.Profile,
		data,
		key,
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

// confirmManualLoginOverwrite protects an existing stored credential before login
// reads a replacement key. Non-TTY stdin without --no-prompt keeps the legacy
// automation path so the piped key is never consumed by a confirmation prompt.
func confirmManualLoginOverwrite(f *cmdutil.Factory, rt config.Runtime, prompter *cmdutil.Prompt, stderr io.Writer) (bool, error) {
	if prompter != nil && prompter.Yes() {
		// --yes is explicit overwrite consent. Persistence will still reload
		// auth later, so this fast path avoids an unnecessary early file read.
		return false, nil
	}
	if prompter != nil && !prompter.InteractiveIn() && !prompter.NoPrompt() {
		// Preserve the existing piped-key automation path: stdin is the key,
		// not an answer to an overwrite prompt.
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
	// An early load error stops before persistence, so permission findings must
	// be shown here. The returned flag prevents a duplicate warning if login
	// later reloads the same file during persistence.
	cmdutil.WarnPermissionFindings(stderr, presentationPermissionFindings(f, findings))
	if err != nil {
		return authFindingsWarned, err
	}

	if !recordExists || record.APIKey == "" {
		// Match credential lookup: a profile record with no usable key is not
		// an overwrite risk and login may repair it.
		return authFindingsWarned, nil
	}
	if prompter == nil || prompter.NoPrompt() {
		return authFindingsWarned, &cmdutil.AbortError{Message: fmt.Sprintf("stored credential already exists for profile %q; re-run with --yes to overwrite without a prompt", semanticString(f, rt.Profile))}
	}
	if !prompter.Interactive() {
		return authFindingsWarned, &cmdutil.AbortError{Message: "this action needs confirmation; re-run with --yes to proceed without a prompt"}
	}

	question := loginOverwriteQuestion(f, rt.Profile, record)
	confirmed, err := prompter.Confirm(question)
	if err != nil {
		return authFindingsWarned, err
	}
	if !confirmed {
		return authFindingsWarned, &cmdutil.AbortError{Message: "canceled; no changes were made"}
	}
	return authFindingsWarned, nil
}

// loginOverwriteQuestion builds a confirmation prompt using only redacted,
// non-secret key metadata from the stored record.
func loginOverwriteQuestion(f *cmdutil.Factory, profile string, record auth.ProfileAuth) string {
	parts := []string{}
	if record.DisplayID != "" {
		parts = append(parts, "key "+semanticString(f, record.DisplayID))
	}
	if record.KeyName != "" {
		parts = append(parts, "name "+semanticString(f, record.KeyName))
	}
	detail := ""
	if len(parts) > 0 {
		detail = " (" + strings.Join(parts, ", ") + ")"
	}
	return fmt.Sprintf("Overwrite stored API key for profile %q%s?", semanticString(f, profile), detail)
}

// promptLoginRuntime lets an interactive first-time login change the product
// base URL before validation. Treat the accepted prompt value like a flag
// override so API URL derivation and validation stay in one config path.
func promptLoginRuntime(f *cmdutil.Factory, prompter interface {
	TextWithDisplay(string, string, string) (string, error)
}, flags config.FlagOverrides, rt config.Runtime, operation string) (config.Runtime, config.FlagOverrides, error) {
	if rt.BaseURLSource != config.RuntimeValueSourceDefault {
		return rt, flags, nil
	}
	baseURL, err := prompter.TextWithDisplay("Base URL", rt.BaseURL, semanticString(f, rt.BaseURL))
	if err != nil {
		return rt, flags, &UsageError{Operation: operation, Detail: "could not read base URL"}
	}
	if baseURL == rt.BaseURL {
		return rt, flags, nil
	}
	flags.BaseURL = config.OverrideString{Value: baseURL, Set: true}
	rt, err = f.ResolveRuntimeWithFlags(flags, config.ResolveForWrite)
	if err != nil {
		return rt, flags, err
	}
	return rt, flags, nil
}

// readLoginKey keeps the API key input channel unambiguous: piped stdin is
// consumed only as the key, while terminal input uses a secret prompt unless
// prompts are disabled.
func readLoginKey(prompter interface {
	InteractiveIn() bool
	NoPrompt() bool
	ReadPiped() (string, error)
	Secret(string) (string, error)
}, operation string) (string, error) {
	var raw string
	var err error
	switch {
	case !prompter.InteractiveIn():
		raw, err = prompter.ReadPiped()
	case prompter.NoPrompt():
		return "", &UsageError{Operation: operation, Detail: "pipe the API key through stdin when --no-prompt is set"}
	default:
		raw, err = prompter.Secret("API key")
	}
	if err != nil {
		return "", &UsageError{Operation: operation, Detail: "could not read API key"}
	}

	key := strings.TrimSpace(raw)
	if key == "" {
		return "", &UsageError{Operation: operation, Detail: "API key must not be empty"}
	}
	if strings.IndexFunc(key, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0 {
		return "", &UsageError{Operation: operation, Detail: "API key must be a single line without whitespace or control characters"}
	}
	return key, nil
}

func envNonBlank(f *cmdutil.Factory, name string) bool {
	environment := lookupEnvironmentValue(f, name)
	return environment.present && environment.value != ""
}
