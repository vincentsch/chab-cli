package authcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
)

const setupRecovery = `Recovery: replace the stored credential with "chab login --api-key --no-prompt --yes < api-key.txt".`

// NewSetupCommand builds the guided manual-key setup coordinator.
func NewSetupCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Set up a verified API or guest trial profile",
		Long: `Set up and verify the selected profile with a copied team API key or
browser-issued guest trial credential.

Team API keys are created and revoked in the product web app. Guest
trial credentials are issued and revoked in the browser trial UI. Guest
trial credentials support only the advertised free operations and expire;
they are not hosted MCP OAuth tokens. An explicit
missing profile name is created and selected. Setup reuses a stored key when
possible, verifies it with one GET /v1/me request, and does not rewrite
auth state when that check succeeds. It makes only the config repair needed to
reproduce the resolved profile and connection settings later.

Piped stdin:
  Stored key state | Piped stdin behavior | Result
  --- | --- | ---
  No usable stored key | Read the pipe as the copied key | Validate, persist, select the profile, and report ready
  Stored key passes /v1/me | Never read the pipe | Reuse the identity, make only any required config repair, and report ready
  Stored key returns an authentication-class failure | Never read the pipe | Use the replacement policy below; a non-interactive invocation returns that API error with explicit recovery guidance

Only an interactive terminal may replace a stored key rejected for
authentication. --yes accepts that overwrite confirmation inside an
interactive setup. --no-prompt disables replacement even with --yes and even
on a terminal; setup returns the authentication error and the recovery command
instead. Other API, network, protocol, authorization, and rate-limit failures
never offer replacement. Exit status 3 identifies the authentication failure.
For explicit scripted replacement, run:
  chab login --api-key --no-prompt --yes < api-key.txt

If CHAB_API_KEY is set, setup still validates the stored or copied profile key,
then warns after successful output that the environment credential takes
precedence for ordinary authenticated commands.

Output is human-readable even when --json is inherited from the root. Use
--plain for copy-safe setup result rows. jq, template, response metadata,
and dry-run output are not available.

Related commands:
  chab login
  chab auth status
  chab doctor`,
		Example: `  chab setup
  chab setup --profile staging --base-url https://example.test
  chab setup --no-prompt < api-key.txt
  chab setup --plain`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetup(cmd, f)
		},
	}
}

// runSetup keeps credential discovery ahead of stdin. A usable stored key is
// always tried first, and replacement input is considered only after an
// authentication rejection in a genuinely interactive session.
func runSetup(cmd *cobra.Command, f *cmdutil.Factory) error {
	// Capture and register the environment value before prompts or API work so
	// every later diagnostic uses the same redaction scope.
	shadow := lookupEnvironmentShadow(f)
	registerEnvironmentShadow(f, shadow)

	flags := cmdutil.RuntimeFlagOverrides(cmd)
	rt, err := f.ResolveRuntimeWithFlags(flags, config.ResolveForWrite)
	if err != nil {
		return err
	}
	if err := requireDistinctStatePaths(rt.ConfigPath, rt.AuthPath); err != nil {
		return err
	}
	prompter := f.Prompt(cmd)
	if prompter.Interactive() {
		rt, flags, err = promptLoginRuntime(f, prompter, flags, rt, "setup")
		if err != nil {
			return err
		}
	}

	authFile, findings, err := auth.Load(rt.AuthPath)
	authFindingsWarned := len(findings) > 0
	var stored auth.ProfileAuth
	var storedUsable bool
	if err == nil {
		stored, storedUsable = authFile.Profiles[rt.Profile]
		storedUsable = storedUsable && stored.APIKey != ""
		// Permission findings can contain the auth path. Register the stored key
		// first in case user-controlled path text overlaps the credential.
		if storedUsable {
			f.RegisterSecret(stored.APIKey)
		}
	}
	cmdutil.WarnPermissionFindings(cmd.ErrOrStderr(), presentationPermissionFindings(f, findings))
	if err != nil {
		return err
	}

	var data api.WhoamiData
	if !storedUsable {
		// This is the only branch that may read piped input without first asking
		// for interactive replacement consent.
		key, readErr := readLoginKey(prompter, "setup")
		if readErr != nil {
			return readErr
		}
		f.RegisterSecret(key)
		data, err = validateSetupKey(cmd, f, rt, key, shadow)
		if err != nil {
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
			"setup",
			setupConfigPreparation(rt),
			config.WritePreservingShape,
		); err != nil {
			return err
		}
		return writeSetupResult(cmd, f, rt, data, shadow)
	}

	// A stored credential is a readiness probe, not a retried login attempt.
	// Limiting it to one physical request prevents hidden delays and preserves
	// the original failure for the replacement decision below.
	storedCredential := auth.Credential{
		APIKey:    stored.APIKey,
		Source:    auth.SourceAuthFile,
		Profile:   rt.Profile,
		DisplayID: stored.DisplayID,
		KeyName:   stored.KeyName,
	}
	client, err := authAPIClient(f, rt, storedCredential, cmd, shadow, true)
	if err != nil {
		return err
	}
	data, meta, err := client.Whoami(cmd.Context())
	if err == nil {
		if err := validateWhoamiIdentity(data, meta); err != nil {
			return err
		}
		if err := repairSetupConfig(rt); err != nil {
			return err
		}
		return writeSetupResult(cmd, f, rt, data, shadow)
	}
	if !isAuthenticationRejection(err) {
		return err
	}
	if !prompter.Interactive() {
		fmt.Fprintln(cmd.ErrOrStderr(), setupRecovery)
		return err
	}

	// --yes skips this question only after the interactive gate above. It must
	// never turn piped or --no-prompt input into an overwrite path.
	if !prompter.Yes() {
		confirmed, confirmErr := prompter.Confirm(loginOverwriteQuestion(f, rt.Profile, stored))
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			return &cmdutil.AbortError{Message: "canceled; no changes were made"}
		}
	}

	replacement, err := readLoginKey(prompter, "setup")
	if err != nil {
		return err
	}
	f.RegisterSecret(replacement)
	data, err = validateSetupKey(cmd, f, rt, replacement, shadow)
	if err != nil {
		return err
	}
	if err := persistValidatedCredential(
		cmd,
		f,
		rt,
		rt.Profile,
		data,
		replacement,
		authFindingsWarned,
		"setup",
		setupConfigPreparation(rt),
		config.WritePreservingShape,
	); err != nil {
		return err
	}
	return writeSetupResult(cmd, f, rt, data, shadow)
}

// validateSetupKey uses the normal client retry policy for a newly supplied
// key. Stored-key probes use the separate one-attempt path in runSetup.
func validateSetupKey(cmd *cobra.Command, f *cmdutil.Factory, rt config.Runtime, key string, shadow environmentShadow) (api.WhoamiData, error) {
	client, err := authAPIClient(f, rt, auth.Credential{APIKey: key}, cmd, shadow, false)
	if err != nil {
		return api.WhoamiData{}, err
	}
	data, meta, err := client.Whoami(cmd.Context())
	if err != nil {
		return api.WhoamiData{}, err
	}
	if err := validateWhoamiIdentity(data, meta); err != nil {
		return api.WhoamiData{}, err
	}
	return data, nil
}

// isAuthenticationRejection uses the typed API exit class so redacted error
// text cannot accidentally change whether replacement is offered.
func isAuthenticationRejection(err error) bool {
	var apiErr *api.Error
	return errors.As(err, &apiErr) && apiErr.ExitCode() == 3
}

// writeSetupResult renders the validated identity before warning about an
// environment override. A result write failure therefore cannot produce a
// misleading success warning.
func writeSetupResult(cmd *cobra.Command, f *cmdutil.Factory, rt config.Runtime, data api.WhoamiData, shadow environmentShadow) error {
	presentation := presentationWhoami(f, data)
	profile := semanticString(f, rt.Profile)
	apiBaseURL := semanticString(f, rt.APIBaseURL)
	if err := f.WriteCommandResult(cmd, cmdutil.CommandResult{
		Human: cmdutil.HumanOutput{
			Render: func(w io.Writer) {
				renderSetupSuccess(w, profile, apiBaseURL, presentation)
			},
			Plain: func(dataW, prose io.Writer) {
				renderSetupPlain(dataW, prose, profile, apiBaseURL, presentation)
			},
		},
		Supports: cmdutil.OutputSupport{Human: true, Plain: true},
	}); err != nil {
		return err
	}
	warnEnvironmentShadow(cmd, shadow)
	return nil
}

// requireDistinctStatePaths prevents one atomic writer from replacing the
// other state format. It catches lexical aliases, existing file aliases, and
// future collisions below aliased parent directories before setup prompts,
// reads a copied key, or contacts the API.
func requireDistinctStatePaths(configPath, authPath string) error {
	configAbsolute, configErr := filepath.Abs(configPath)
	authAbsolute, authErr := filepath.Abs(authPath)
	if configErr == nil && authErr == nil &&
		filepath.Clean(configAbsolute) == filepath.Clean(authAbsolute) {
		return &UsageError{Operation: "setup", Detail: "config and auth files must use different paths"}
	}

	configIdentity, configIdentityOK := prospectiveStatePath(configPath)
	authIdentity, authIdentityOK := prospectiveStatePath(authPath)
	if configIdentityOK && authIdentityOK &&
		os.SameFile(configIdentity.ancestor, authIdentity.ancestor) &&
		equalStatePathSuffix(configIdentity.suffix, authIdentity.suffix) {
		return &UsageError{Operation: "setup", Detail: "config and auth files must use different paths"}
	}
	return nil
}

type prospectivePathIdentity struct {
	ancestor os.FileInfo
	suffix   string
}

// prospectiveStatePath describes a path by its deepest existing ancestor and
// the still-missing suffix below it. Comparing ancestors catches symlinked or
// bind-mounted parent aliases even when neither final state file exists yet.
func prospectiveStatePath(path string) (prospectivePathIdentity, bool) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return prospectivePathIdentity{}, false
	}
	current := filepath.Clean(absolute)
	var suffix string
	for {
		info, statErr := os.Stat(current)
		if statErr == nil {
			return prospectivePathIdentity{ancestor: info, suffix: suffix}, true
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return prospectivePathIdentity{}, false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return prospectivePathIdentity{}, false
		}
		if suffix == "" {
			suffix = filepath.Base(current)
		} else {
			suffix = filepath.Join(filepath.Base(current), suffix)
		}
		current = parent
	}
}

func equalStatePathSuffix(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
