package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/vincentsch/chab-cli/internal/redact"
)

// settingSource gives api_base_url enough context to decide whether a
// lower-precedence explicit value should survive a higher-precedence base_url.
type settingSource = RuntimeValueSource

const (
	sourceDefault = RuntimeValueSourceDefault
	sourceFile    = RuntimeValueSourceFile
	sourceEnv     = RuntimeValueSourceEnv
	sourceFlag    = RuntimeValueSourceFlag
	sourceDerived = RuntimeValueSourceDerived
)

// ResolveRuntime resolves paths, selected profile, URLs, locale, and carried
// defaults using flag > env > file > default precedence.
func ResolveRuntime(opts Options) (Runtime, error) {
	lookup := opts.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}

	configPath, authPath, err := resolveFilePaths(opts)
	if err != nil {
		return Runtime{}, err
	}

	file, err := LoadWithAuthFallback(configPath, authPath)
	if err != nil {
		return Runtime{}, err
	}

	profileName, profileSource, err := resolveProfileName(file, opts.Flags.Profile, lookup)
	if err != nil {
		return Runtime{}, withConfigContext(err, configPath, "")
	}
	if err := validateProfileName(profileName); err != nil {
		return Runtime{}, withConfigContext(err, configPath, profileName)
	}

	profile, profileExists := file.Profiles[profileName]
	meta := file.profileMeta[profileName]
	if opts.Mode == ResolveStrict && file.exists && !profileExists && !hasURLOverride(opts.Flags, lookup) {
		// Strict mode only needs the selected profile when no URL override makes
		// the runtime self-contained. This keeps env-only CI flows independent
		// from local profile files.
		return Runtime{}, newError(ErrMissingProfile, configPath, profileName, "profile", "profile is not present in config file", nil)
	}

	baseURL, baseSource, err := resolveBaseURL(configPath, profileName, profile, meta, profileExists || file.legacyAuthProfiles[profileName], opts.Flags.BaseURL, lookup)
	if err != nil {
		return Runtime{}, err
	}
	apiBaseURL, apiBaseSource, err := resolveAPIBaseURL(configPath, profileName, profile, meta, profileExists, opts.Flags.APIBaseURL, lookup, baseURL, baseSource)
	if err != nil {
		return Runtime{}, err
	}
	locale, err := resolveLocale(configPath, profileName, profile, meta, profileExists, opts.Flags.Locale, lookup)
	if err != nil {
		return Runtime{}, err
	}
	defaultOutput, err := resolveDefaultOutput(configPath, profileName, profile, meta, profileExists)
	if err != nil {
		return Runtime{}, err
	}
	projectListLimit, err := resolveProjectListLimit(configPath, profileName, profile, meta, profileExists)
	if err != nil {
		return Runtime{}, err
	}

	return Runtime{
		Profile:          profileName,
		ProfileSource:    profileSource,
		ConfigPath:       configPath,
		AuthPath:         authPath,
		BaseURL:          baseURL,
		BaseURLSource:    baseSource,
		APIBaseURL:       apiBaseURL,
		APIBaseURLSource: apiBaseSource,
		Locale:           locale,
		DefaultOutput:    defaultOutput,
		ProjectListLimit: projectListLimit,
	}, nil
}

// Paths is the resolved config and auth file paths for one invocation.
type Paths struct {
	ConfigPath string
	AuthPath   string
}

// ResolvePaths resolves local config and auth paths without loading either
// file. It honors flag > env > default precedence and does not create state.
func ResolvePaths(opts Options) (Paths, error) {
	configPath, authPath, err := resolveFilePaths(opts)
	if err != nil {
		return Paths{}, err
	}
	return Paths{ConfigPath: configPath, AuthPath: authPath}, nil
}

func resolveFilePaths(opts Options) (configPath, authPath string, err error) {
	lookup := opts.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}

	userConfigDir := opts.UserConfigDir
	if userConfigDir == nil {
		userConfigDir = os.UserConfigDir
	}

	defaultDir := ""
	pathDefault := func() (string, error) {
		// Resolve the user config directory lazily and only once. Commands that
		// set both paths explicitly should not touch process home-directory state.
		if defaultDir != "" {
			return defaultDir, nil
		}
		dir, dirErr := userConfigDir()
		if dirErr != nil {
			return "", newError(ErrMalformedConfig, "", "", "user_config_dir", "could not determine user config directory", dirErr)
		}
		if dir == "" {
			return "", newError(ErrMalformedConfig, "", "", "user_config_dir", "must not be empty", nil)
		}
		defaultDir = dir
		return defaultDir, nil
	}

	configPath, err = resolvePath("config", opts.Flags.ConfigPath, "CHAB_CONFIG", lookup, pathDefault, "config.yml")
	if err != nil {
		return "", "", err
	}
	authPath, err = resolvePath("auth_file", opts.Flags.AuthPath, "CHAB_AUTH_FILE", lookup, pathDefault, "auth.json")
	if err != nil {
		return "", "", err
	}
	return configPath, authPath, nil
}

// ValidateProfileName validates a profile name against the runtime rule.
func ValidateProfileName(name string) error {
	return validateProfileName(name)
}

// SelectedProfile returns the profile selected by flag > env > file > default
// precedence. It validates the selected name but does not require it to exist.
func SelectedProfile(file *File, flags FlagOverrides, lookupEnv func(string) (string, bool)) (string, error) {
	if file == nil {
		file = newFile(false)
	}
	lookup := lookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	name, _, err := resolveProfileName(file, flags.Profile, lookup)
	if err != nil {
		return "", err
	}
	if err := validateProfileName(name); err != nil {
		return "", err
	}
	return name, nil
}

func resolvePath(field string, flag OverrideString, envName string, lookup func(string) (string, bool), defaultDir func() (string, error), filename string) (string, error) {
	if flag.Set {
		if flag.Value == "" {
			return "", newError(ErrMalformedConfig, "", "", field, "path must not be empty", nil)
		}
		return flag.Value, nil
	}
	if value, ok := lookupNonBlank(lookup, envName); ok {
		return value, nil
	}
	dir, err := defaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "chab", filename), nil
}

// resolveProfileName returns both the winning profile name and the source that
// selected it. The name alone is not enough for login because only flag/env
// selections are exact write targets.
func resolveProfileName(file *File, flag OverrideString, lookup func(string) (string, bool)) (string, ProfileSource, error) {
	if flag.Set {
		if flag.Value == "" {
			return "", ProfileSourceDefault, newError(ErrInvalidProfileName, "", "", "profile", "must not be empty", nil)
		}
		return flag.Value, ProfileSourceFlag, nil
	}
	if value, ok := lookupNonBlank(lookup, "CHAB_PROFILE"); ok {
		return value, ProfileSourceEnv, nil
	}
	if file.CurrentProfile != "" {
		return file.CurrentProfile, ProfileSourceCurrent, nil
	}
	return DefaultProfile, ProfileSourceDefault, nil
}

func resolveBaseURL(path, profileName string, profile Profile, meta profileMetadata, profileExists bool, flag OverrideString, lookup func(string) (string, bool)) (string, settingSource, error) {
	value := DefaultBaseURL
	source := sourceDefault
	if profileExists {
		value = LegacyImplicitBaseURL
	}

	if profileExists && meta.BaseURLSet {
		value = profile.BaseURL
		source = sourceFile
	}
	if envValue, ok := lookupNonBlank(lookup, "CHAB_BASE_URL"); ok {
		value = envValue
		source = sourceEnv
	}
	if flag.Set {
		value = flag.Value
		source = sourceFlag
	}

	normalized, err := normalizeURL("base_url", value)
	if err != nil {
		return "", source, withConfigContext(err, path, profileName)
	}
	return normalized, source, nil
}

func resolveAPIBaseURL(path, profileName string, profile Profile, meta profileMetadata, profileExists bool, flag OverrideString, lookup func(string) (string, bool), baseURL string, baseSource settingSource) (string, settingSource, error) {
	value := deriveAPIBaseURL(baseURL)
	source := sourceDerived

	if profileExists && meta.APIBaseURLSet {
		value = profile.APIBaseURL
		source = sourceFile
	}
	if envValue, ok := lookupNonBlank(lookup, "CHAB_API_BASE_URL"); ok {
		value = envValue
		source = sourceEnv
	}
	if flag.Set {
		value = flag.Value
		source = sourceFlag
	}

	// Explicit API URL sources win outright. Otherwise a base_url from a
	// higher-precedence source invalidates a file/default api_base_url, so the
	// API URL is derived again from the winning base_url.
	if source != sourceFlag && source != sourceEnv {
		if source != sourceFile || baseURLOverridesFileAPI(baseSource) {
			value = deriveAPIBaseURL(baseURL)
			source = sourceDerived
		}
	}

	normalized, err := normalizeURL("api_base_url", value)
	if err != nil {
		return "", source, withConfigContext(err, path, profileName)
	}
	return normalized, source, nil
}

func baseURLOverridesFileAPI(source settingSource) bool {
	return source == sourceEnv || source == sourceFlag
}

func resolveLocale(path, profileName string, profile Profile, meta profileMetadata, profileExists bool, flag OverrideString, lookup func(string) (string, bool)) (string, error) {
	value := ""
	if profileExists && meta.LocaleSet {
		value = profile.Locale
	}
	if envValue, ok := lookupNonBlank(lookup, "CHAB_LOCALE"); ok {
		value = envValue
	}
	if flag.Set {
		if flag.Value == "" {
			return "", withConfigContext(newError(ErrInvalidLocale, "", "", "locale", "must not be empty when set by flag", nil), path, profileName)
		}
		value = flag.Value
	}
	if err := validateLocale(value); err != nil {
		return "", withConfigContext(err, path, profileName)
	}
	return value, nil
}

func resolveDefaultOutput(path, profileName string, profile Profile, meta profileMetadata, profileExists bool) (string, error) {
	value := DefaultOutput
	if profileExists && meta.DefaultOutputSet {
		value = profile.DefaultOutput
	}
	if err := validateDefaultOutput(value); err != nil {
		return "", withConfigContext(err, path, profileName)
	}
	return value, nil
}

func resolveProjectListLimit(path, profileName string, profile Profile, meta profileMetadata, profileExists bool) (int, error) {
	value := DefaultProjectListLimit
	if profileExists && meta.ProjectListLimitSet {
		value = profile.Defaults.ProjectListLimit
	}
	if err := validateProjectListLimit(value); err != nil {
		return 0, withConfigContext(err, path, profileName)
	}
	return value, nil
}

func hasURLOverride(flags FlagOverrides, lookup func(string) (string, bool)) bool {
	if flags.BaseURL.Set || flags.APIBaseURL.Set {
		return true
	}
	if _, ok := lookupNonBlank(lookup, "CHAB_BASE_URL"); ok {
		return true
	}
	if _, ok := lookupNonBlank(lookup, "CHAB_API_BASE_URL"); ok {
		return true
	}
	return false
}

func lookupNonBlank(lookup func(string) (string, bool), name string) (string, bool) {
	value, ok := lookup(name)
	if !ok || value == "" {
		return "", false
	}
	return value, true
}

// withConfigContext fills in path/profile context at the boundary where the
// failing helper did not know which file/profile was being resolved. It
// returns a copy so the original error is not mutated with boundary-specific
// context.
func withConfigContext(err error, path, profile string) error {
	var configErr *Error
	if !errors.As(err, &configErr) {
		return err
	}
	copyErr := *configErr
	if copyErr.Path == "" {
		copyErr.Path = redact.String(path)
	}
	if copyErr.Profile == "" {
		copyErr.Profile = redact.String(profile)
	}
	return &copyErr
}
