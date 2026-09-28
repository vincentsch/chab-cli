package profile

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/auth"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/redact"
)

type usageError struct {
	detail string
}

func (e *usageError) Error() string {
	if e == nil {
		return "profile command error"
	}
	return "profile command error: " + e.detail
}

func (e *usageError) ExitCode() int {
	return 1
}

// storedAuthJSON is the only auth information profile/config commands expose.
// It deliberately mirrors non-secret auth metadata and has no field for APIKey.
type storedAuthJSON struct {
	Present         bool       `json:"present"`
	DisplayID       *string    `json:"display_id"`
	KeyName         *string    `json:"key_name"`
	TeamDisplayID   *string    `json:"team_display_id"`
	TeamName        *string    `json:"team_name"`
	LastValidatedAt *time.Time `json:"last_validated_at"`
}

// profileJSON is the stable machine shape shared by profile list and profile
// show. Keeping a single shape makes jq/template use consistent across both
// commands.
type profileJSON struct {
	Name             string         `json:"name"`
	Selected         bool           `json:"selected"`
	Persisted        bool           `json:"persisted"`
	BaseURL          string         `json:"base_url"`
	APIBaseURL       string         `json:"api_base_url"`
	Locale           string         `json:"locale"`
	DefaultOutput    string         `json:"default_output"`
	ProjectListLimit int            `json:"project_list_limit"`
	StoredAuth       storedAuthJSON `json:"stored_auth"`
}

// profileListJSON is the stable machine value for profile list. Paths are
// included so scripts can diagnose which local files were inspected.
type profileListJSON struct {
	CurrentProfile string        `json:"current_profile"`
	ConfigPath     string        `json:"config_path"`
	AuthPath       string        `json:"auth_path"`
	Profiles       []profileJSON `json:"profiles"`
}

// configPathJSON is the stable machine value for config path. It is produced
// without loading either file, so it remains available when config.yml is bad.
type configPathJSON struct {
	ConfigPath string `json:"config_path"`
	AuthPath   string `json:"auth_path"`
}

// configValueJSON is the stable machine value for one known config key.
type configValueJSON struct {
	Key    string `json:"key"`
	Value  any    `json:"value"`
	Source string `json:"source"`
}

// configListJSON is the stable machine value for config list.
type configListJSON struct {
	ConfigPath string            `json:"config_path"`
	Values     []configValueJSON `json:"values"`
}

func toConfigValueJSON(value config.ConfigValue) configValueJSON {
	return configValueJSON{Key: value.Key, Value: value.Value, Source: value.Source}
}

// storedAuthFor builds the public, non-secret auth view for one profile. Each
// record is sanitized independently so its API key never becomes shared
// command state or part of the returned model.
func storedAuthFor(file *auth.File, name string) storedAuthJSON {
	if file == nil {
		return storedAuthJSON{Present: false}
	}
	record, ok := file.Profiles[name]
	if !ok {
		return storedAuthJSON{Present: false}
	}
	// Use a short-lived, command-local registry so a stored key copied into
	// user-controlled metadata cannot reach any renderer or transform. The key
	// itself never enters the result model or the invocation-wide registry.
	secrets := redact.NewRegistry()
	secrets.RegisterSecret(record.APIKey)
	displayID := secrets.RedactValue(record.DisplayID)
	keyName := secrets.RedactValue(record.KeyName)
	teamDisplayID := secrets.RedactValue(record.TeamDisplayID)
	teamName := secrets.RedactValue(record.TeamName)
	return storedAuthJSON{
		Present:         true,
		DisplayID:       &displayID,
		KeyName:         &keyName,
		TeamDisplayID:   &teamDisplayID,
		TeamName:        &teamName,
		LastValidatedAt: record.LastValidatedAt,
	}
}

// buildProfileJSON resolves and validates config before attaching sanitized
// stored-auth metadata, producing the shared list/show machine shape.
func buildProfileJSON(file *config.File, authFile *auth.File, name, current string) (profileJSON, error) {
	// ResolveProfileView validates persisted known values before we attach auth
	// metadata, so no output mode can emit invalid config as if it were usable.
	view, persisted, err := config.ResolveProfileView(file, name)
	if err != nil {
		return profileJSON{}, err
	}
	return profileJSON{
		Name:             name,
		Selected:         name == current,
		Persisted:        persisted,
		BaseURL:          view.BaseURL,
		APIBaseURL:       view.APIBaseURL,
		Locale:           view.Locale,
		DefaultOutput:    view.DefaultOutput,
		ProjectListLimit: view.ProjectListLimit,
		StoredAuth:       storedAuthFor(authFile, name),
	}, nil
}

func loadConfig(cmd *cobra.Command, f *cmdutil.Factory) (*config.File, config.Paths, error) {
	paths, err := f.ResolveLocalPaths(cmd)
	if err != nil {
		return nil, config.Paths{}, err
	}
	file, err := config.LoadWithAuthFallback(paths.ConfigPath, paths.AuthPath)
	if err != nil {
		return nil, config.Paths{}, err
	}
	return file, paths, nil
}

// loadConfigAndAuth is the shared local-state path for commands that need both
// stores. Config is loaded first, then auth permission warnings and data.
func loadConfigAndAuth(cmd *cobra.Command, f *cmdutil.Factory) (*config.File, *auth.File, config.Paths, error) {
	file, paths, err := loadConfig(cmd, f)
	if err != nil {
		return nil, nil, config.Paths{}, err
	}
	authFile, err := loadAuth(cmd, paths)
	if err != nil {
		return nil, nil, config.Paths{}, err
	}
	return file, authFile, paths, nil
}

// loadAuth reports permission findings even when parsing later fails, so users
// do not lose an actionable file-permission warning behind the load error.
func loadAuth(cmd *cobra.Command, paths config.Paths) (*auth.File, error) {
	authFile, findings, err := auth.Load(paths.AuthPath)
	// Permission findings are warnings, not data. Emit them before returning so
	// every command that inspects stored auth reports broad file permissions.
	cmdutil.WarnPermissionFindings(cmd.ErrOrStderr(), findings)
	if err != nil {
		return nil, err
	}
	return authFile, nil
}
