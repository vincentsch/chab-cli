package cmdutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
	"github.com/vincentsch/chab-cli/internal/prompt"
)

// SecretRegistry is the exact-value redaction surface shared by one command
// invocation. Rungrad remains a compatibility field for retained callers.
type SecretRegistry interface {
	RegisterSecret(string)
	RedactJSON([]byte) []byte
	RedactText([]byte) []byte
}

// BrowserOpener opens a browser or operating-system approval UI for a login
// URL. Production wiring owns the concrete launcher; tests inject a fake.
type BrowserOpener func(context.Context, string) error

// BootstrapClient is the narrow unauthenticated CLI-auth surface login uses.
type BootstrapClient interface {
	RegisterSecret(string)
	DeviceEndpoint() string
	TokenEndpoint() string
	Compatibility(context.Context, string) (api.CompatibilityData, api.ResponseMeta, error)
	CreateDevice(context.Context, string, string, []string, string) (api.DeviceData, api.ResponseMeta, error)
	PollToken(context.Context, string) (api.TokenData, api.ResponseMeta, error)
}

// BootstrapClientFactory creates a credential-free CLI-auth client for one
// resolved runtime. The root composes the production API implementation.
type BootstrapClientFactory func(config.Runtime, *cobra.Command) (BootstrapClient, error)

// Factory carries the process-dependent inputs commands need, injected once by
// internal/cli so command packages read no globals directly.
type Factory struct {
	Stdin            io.Reader
	StdinIsTerminal  func() bool
	StdoutIsTerminal func() bool
	TerminalHeight   func() (int, bool)
	RunPager         PagerRunner
	LookupEnv        func(string) (string, bool)
	Sleeper          api.Sleeper
	Now              func() time.Time
	Version          string
	Secrets          SecretRegistry
	Rungrad          SecretRegistry
	BrowserOpener    BrowserOpener
	BootstrapFactory BootstrapClientFactory
}

// CredentialState is the read-only credential/report view used by auth status
// and doctor. Source is empty when no usable credential exists; Record is
// populated only when the selected profile has an auth-file entry that was
// inspected.
type CredentialState struct {
	Source     auth.CredentialSource
	APIKey     string
	Credential auth.Credential
	Record     *auth.ProfileAuth
	Findings   []auth.PermissionFinding
	FileExists bool
}

// ResolveRuntime wraps config.ResolveRuntime with parsed flags and injected env
// lookup.
func (f *Factory) ResolveRuntime(cmd *cobra.Command, mode config.ResolveMode) (config.Runtime, error) {
	return f.ResolveRuntimeWithFlags(RuntimeFlagOverrides(cmd), mode)
}

// ResolveRuntimeWithFlags resolves runtime values from an explicit override
// set. Commands use this after interactive prompts adjust profile or base URL.
func (f *Factory) ResolveRuntimeWithFlags(flags config.FlagOverrides, mode config.ResolveMode) (config.Runtime, error) {
	return config.ResolveRuntime(config.Options{
		Flags:     flags,
		LookupEnv: f.lookupEnv,
		Mode:      mode,
	})
}

// ResolveLocalPaths resolves config/auth file paths from parsed flags and env
// without loading either file.
func (f *Factory) ResolveLocalPaths(cmd *cobra.Command) (config.Paths, error) {
	return config.ResolvePaths(config.Options{
		Flags:     RuntimeFlagOverrides(cmd),
		LookupEnv: f.lookupEnv,
	})
}

// SelectedProfile resolves the selected profile name for a loaded config file.
func (f *Factory) SelectedProfile(cmd *cobra.Command, file *config.File) (string, error) {
	return config.SelectedProfile(file, RuntimeFlagOverrides(cmd), f.lookupEnv)
}

// Credential wraps auth.LookupCredential.
func (f *Factory) Credential(rt config.Runtime) (auth.Credential, []auth.PermissionFinding, error) {
	cred, findings, err := auth.LookupCredential(rt, auth.Options{LookupEnv: f.lookupEnv})
	if err == nil {
		f.RegisterSecret(cred.APIKey)
	}
	return cred, findings, err
}

// RegisterSecret marks a value as sensitive for every output mode and the
// returned error of the current invocation.
func (f *Factory) RegisterSecret(value string) {
	if registry := f.secretRegistry(); registry != nil {
		registry.RegisterSecret(value)
	}
}

// RedactValue replaces exact secrets in one data value before it is handed to
// renderers. This preserves output labels and JSON structure that merely share
// the same bytes as a secret.
func (f *Factory) RedactValue(value string) string {
	registry := f.secretRegistry()
	redactor, ok := registry.(interface{ RedactValue(string) string })
	if !ok {
		return value
	}
	return redactor.RedactValue(value)
}

// RedactJSON removes exact registered secrets from JSON string values before a
// caller exposes already-encoded server data.
func (f *Factory) RedactJSON(data []byte) []byte {
	registry := f.secretRegistry()
	if registry == nil {
		return append([]byte(nil), data...)
	}
	return registry.RedactJSON(data)
}

// RedactError preserves error classification and unwrapping while replacing
// exact registered values and terminal controls in its public message.
func (f *Factory) RedactError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	registry := f.secretRegistry()
	if registry != nil {
		message = string(registry.RedactText([]byte(message)))
	}
	message = string(output.SanitizeControlBytes([]byte(message)))
	if message == err.Error() {
		return err
	}
	return &redactedCommandError{message: message, err: err}
}

// secretRegistry prefers the active command's per-invocation registry. The
// fallback keeps older callers working without sharing secrets across runs.
func (f *Factory) secretRegistry() SecretRegistry {
	if f == nil {
		return nil
	}
	if f.Secrets != nil {
		return f.Secrets
	}
	return f.Rungrad
}

// redactedCommandError changes only the public message. Unwrapping still
// reaches the original typed error for exit-code and API-error classification.
type redactedCommandError struct {
	message string
	err     error
}

func (e *redactedCommandError) Error() string { return e.message }
func (e *redactedCommandError) Unwrap() error { return e.err }

// APIClient builds an API client through the shared runtime construction path.
func (f *Factory) APIClient(rt config.Runtime, cred auth.Credential, cmd *cobra.Command) (*api.Client, error) {
	return f.apiClient(rt, cred, cmd, nil)
}

// APIClientNoRetry builds an API client that performs one physical request per
// command-level call. Readiness probes use this for strict "at most one" call
// contracts.
func (f *Factory) APIClientNoRetry(rt config.Runtime, cred auth.Credential, cmd *cobra.Command) (*api.Client, error) {
	return f.apiClient(rt, cred, cmd, func(opts *api.Options) {
		opts.MaxAttempts = 1
	})
}

// APIClientConfigured builds an API client through the shared runtime path and
// applies a caller hook to the options. Use it for per-invocation API settings
// such as raw API secret redaction, not for replacing runtime resolution.
func (f *Factory) APIClientConfigured(rt config.Runtime, cred auth.Credential, cmd *cobra.Command, configure func(*api.Options)) (*api.Client, error) {
	return f.apiClient(rt, cred, cmd, configure)
}

// PublicAPIClient builds a credential-free API client for documented public
// /v1 endpoints such as health, errors, and CLI compatibility.
func (f *Factory) PublicAPIClient(rt config.Runtime, cmd *cobra.Command) (*api.Client, error) {
	opts := api.Options{
		BaseURL:          rt.APIBaseURL,
		Locale:           rt.Locale,
		UserAgentVersion: f.VersionString(),
		Sleeper:          f.sleeper(),
		Now:              f.Clock(),
		AllowNoAuth:      true,
	}
	if DebugEnabled(cmd) {
		opts.DebugWriter = cmd.ErrOrStderr()
	}
	return api.New(opts)
}

func (f *Factory) apiClient(rt config.Runtime, cred auth.Credential, cmd *cobra.Command, configure func(*api.Options)) (*api.Client, error) {
	opts := api.OptionsFromRuntime(rt, cred, f.VersionString(), DebugEnabled(cmd), cmd.ErrOrStderr())
	opts.Sleeper = f.sleeper()
	opts.Now = f.Clock()
	if configure != nil {
		configure(&opts)
	}
	// Hooks add command-specific options, but credential redaction is a factory
	// invariant even if a hook replaces the secret-value slice.
	opts.SecretValues = append(opts.SecretValues, cred.APIKey)
	return api.New(opts)
}

func (f *Factory) VersionString() string {
	if f != nil && f.Version != "" {
		return f.Version
	}
	return "dev"
}

func (f *Factory) Clock() func() time.Time {
	if f != nil && f.Now != nil {
		return f.Now
	}
	return time.Now
}

func (f *Factory) Sleep(ctx context.Context, d time.Duration) error {
	return f.sleeper().Sleep(ctx, d)
}

// OpenBrowser invokes the injected browser opener.
func (f *Factory) OpenBrowser(ctx context.Context, rawURL string) error {
	if f != nil && f.BrowserOpener != nil {
		return f.BrowserOpener(ctx, rawURL)
	}
	return fmt.Errorf("browser opener is not configured")
}

// BootstrapClient creates the credential-free CLI-auth client for browser
// login. A missing factory is a local setup error rather than an implicit
// network fallback.
func (f *Factory) BootstrapClient(rt config.Runtime, cmd *cobra.Command) (BootstrapClient, error) {
	if f != nil && f.BootstrapFactory != nil {
		return f.BootstrapFactory(rt, cmd)
	}
	return nil, fmt.Errorf("bootstrap client is not configured")
}

// Prompter returns a prompt helper over injected stdin and command stderr.
func (f *Factory) Prompter(cmd *cobra.Command) *prompt.Prompter {
	return prompt.New(f.stdin(), cmd.ErrOrStderr(), f.stdinIsTerminal())
}

// Prompt returns the policy-aware prompt surface, binding the injected prompter
// to parsed --no-prompt and --yes state for the current command invocation.
func (f *Factory) Prompt(cmd *cobra.Command) *Prompt {
	return &Prompt{
		prompter: f.Prompter(cmd),
		noPrompt: NoPromptEnabled(cmd),
		yes:      YesEnabled(cmd),
	}
}

// CredentialState inspects selected credential state without mutating local
// files. It preserves CHAB_API_KEY precedence, while inspectAuthFile lets
// doctor still report auth-file existence and permission findings when an env
// credential is active or no credential exists.
func (f *Factory) CredentialState(rt config.Runtime, inspectAuthFile bool) (CredentialState, error) {
	var state CredentialState
	cred, findings, err := f.Credential(rt)
	if err != nil {
		if !isMissingCredential(err) {
			state.Findings = findings
			return state, err
		}
		if inspectAuthFile {
			// Missing credentials are not fatal for status/doctor. When doctor
			// asks for local auth inspection, load the file separately so a
			// malformed or broadly-permissioned file is still visible.
			file, loadFindings, loadErr := auth.Load(rt.AuthPath)
			if loadErr != nil {
				state.Findings = loadFindings
				return state, loadErr
			}
			state.Findings = loadFindings
			state.FileExists = file.Exists()
			if record, ok := file.Profiles[rt.Profile]; ok {
				state.Record = &record
			}
		}
		return state, nil
	}

	state.Source = cred.Source
	state.APIKey = cred.APIKey
	state.Credential = cred
	state.Findings = findings

	if cred.Source == auth.SourceAuthFile || inspectAuthFile {
		// LookupCredential intentionally returns only the secret-bearing
		// credential. Reporting commands reload the auth file to attach
		// non-secret cached metadata and permission findings.
		file, loadFindings, loadErr := auth.Load(rt.AuthPath)
		if loadErr != nil {
			state.Findings = loadFindings
			return state, loadErr
		}
		state.Findings = loadFindings
		state.FileExists = file.Exists()
		if record, ok := file.Profiles[rt.Profile]; ok {
			state.Record = &record
		}
	}
	return state, nil
}

func (f *Factory) stdin() io.Reader {
	if f == nil || f.Stdin == nil {
		return os.Stdin
	}
	return f.Stdin
}

func (f *Factory) stdinIsTerminal() bool {
	if f != nil && f.StdinIsTerminal != nil {
		return f.StdinIsTerminal()
	}
	return prompt.IsTerminalReader(f.stdin())
}

func (f *Factory) stdoutIsTerminal() bool {
	if f != nil && f.StdoutIsTerminal != nil {
		return f.StdoutIsTerminal()
	}
	return false
}

func (f *Factory) terminalHeight() (int, bool) {
	if f != nil && f.TerminalHeight != nil {
		return f.TerminalHeight()
	}
	return 0, false
}

func (f *Factory) pagerRunner() PagerRunner {
	if f != nil && f.RunPager != nil {
		return f.RunPager
	}
	return DefaultPagerRunner
}

func (f *Factory) lookupEnv(name string) (string, bool) {
	if f != nil && f.LookupEnv != nil {
		return f.LookupEnv(name)
	}
	return os.LookupEnv(name)
}

func (f *Factory) sleeper() api.Sleeper {
	if f != nil && f.Sleeper != nil {
		return f.Sleeper
	}
	return api.RealSleeper()
}

func isMissingCredential(err error) bool {
	var authErr *auth.Error
	return errors.As(err, &authErr) && authErr.Kind == auth.ErrMissingCredential
}
