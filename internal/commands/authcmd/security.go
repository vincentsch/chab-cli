package authcmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/output"
)

const environmentShadowMinimumLength = 8

// environmentShadow captures one CHAB_API_KEY lookup. Non-empty values always
// trigger a warning, while only sufficiently specific forms become exact
// redaction values.
type environmentShadow struct {
	original string
	warn     bool
	forms    []string
}

// environmentValue distinguishes an unset variable from a present empty value.
type environmentValue struct {
	value   string
	present bool
}

// lookupEnvironmentValue centralizes nil-safe access to the injected
// environment lookup.
func lookupEnvironmentValue(f *cmdutil.Factory, name string) environmentValue {
	if f == nil || f.LookupEnv == nil {
		return environmentValue{}
	}
	value, present := f.LookupEnv(name)
	return environmentValue{value: value, present: present}
}

// lookupEnvironmentShadow reads CHAB_API_KEY exactly once and prepares both
// its warning state and safe exact-redaction forms.
func lookupEnvironmentShadow(f *cmdutil.Factory) environmentShadow {
	environment := lookupEnvironmentValue(f, "CHAB_API_KEY")
	state := environmentShadow{
		original: environment.value,
		warn:     environment.present && environment.value != "",
	}
	trimmed := strings.TrimSpace(environment.value)
	// Short values such as common words would erase unrelated output if used
	// as global exact replacements. They still produce the shadow warning.
	if !environment.present || len(trimmed) < environmentShadowMinimumLength {
		return state
	}
	state.forms = append(state.forms, environment.value)
	if trimmed != environment.value {
		state.forms = append(state.forms, trimmed)
	}
	return state
}

// registerEnvironmentShadow marks the environment value as secret before
// prompts, permission warnings, or results can print overlapping text.
func registerEnvironmentShadow(f *cmdutil.Factory, state environmentShadow) {
	for _, value := range state.forms {
		f.RegisterSecret(value)
	}
}

// mergeEnvironmentShadow extends API diagnostic redaction without duplicating
// secret values already registered by the client factory.
func mergeEnvironmentShadow(opts *api.Options, state environmentShadow) {
	seen := make(map[string]struct{}, len(opts.SecretValues)+len(state.forms))
	for _, value := range opts.SecretValues {
		seen[value] = struct{}{}
	}
	for _, value := range state.forms {
		if _, ok := seen[value]; ok {
			continue
		}
		opts.SecretValues = append(opts.SecretValues, value)
		seen[value] = struct{}{}
	}
}

// authAPIClient applies the shared environment redaction scope and optionally
// caps a readiness probe at one physical request.
func authAPIClient(f *cmdutil.Factory, rt config.Runtime, cred auth.Credential, cmd *cobra.Command, shadow environmentShadow, oneAttempt bool) (*api.Client, error) {
	return f.APIClientConfigured(rt, cred, cmd, func(opts *api.Options) {
		mergeEnvironmentShadow(opts, shadow)
		if oneAttempt {
			opts.MaxAttempts = 1
		}
	})
}

// semanticString removes key-shaped values and secrets already known to this
// command before the string reaches a prompt, warning, or result.
func semanticString(f *cmdutil.Factory, value string) string {
	value = auth.RedactString(value)
	if f != nil {
		value = f.RedactValue(value)
	}
	return output.SanitizeInlineText(value)
}

// presentationWhoami deep-copies nested slices and pointers before redacting
// display fields. The original response remains available for auth metadata
// persistence and must never be replaced with presentation markers.
func presentationWhoami(f *cmdutil.Factory, data api.WhoamiData) api.WhoamiData {
	out := data
	out.PrincipalType = semanticString(f, data.PrincipalType)
	out.PrincipalID = semanticString(f, data.PrincipalID)
	out.GuestID = semanticString(f, data.GuestID)
	out.TokenID = semanticString(f, data.TokenID)
	out.TokenPublicID = semanticString(f, data.TokenPublicID)
	out.RequestID = semanticString(f, data.RequestID)
	out.Scopes = append([]string(nil), data.Scopes...)
	for index := range out.Scopes {
		out.Scopes[index] = semanticString(f, data.Scopes[index])
	}
	out.TokenControls.FeatureAccess.Mode = semanticString(f, data.TokenControls.FeatureAccess.Mode)
	out.TokenControls.Spending.Mode = semanticString(f, data.TokenControls.Spending.Mode)
	out.TokenControls.ProjectAccess.Mode = semanticString(f, data.TokenControls.ProjectAccess.Mode)
	out.TokenControls.ProjectAccess.SelectedProjectIDs = append([]int64(nil), data.TokenControls.ProjectAccess.SelectedProjectIDs...)
	return out
}

// presentationPermissionFindings redacts copied path values without changing
// the findings returned by auth loading.
func presentationPermissionFindings(f *cmdutil.Factory, findings []auth.PermissionFinding) []auth.PermissionFinding {
	out := append([]auth.PermissionFinding(nil), findings...)
	for index := range out {
		out[index].Path = semanticString(f, findings[index].Path)
	}
	return out
}

// warnEnvironmentShadow is called only after successful result rendering.
func warnEnvironmentShadow(cmd *cobra.Command, state environmentShadow) {
	if state.warn {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: CHAB_API_KEY is set and takes precedence over the stored profile credential.")
	}
}
