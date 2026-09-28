package config

import (
	"sort"
	"strings"
)

const (
	sourceFileLabel    = "file"
	sourceDerivedLabel = "derived"
	sourceDefaultLabel = "default"
)

// ProfileView is the resolved display view of one profile's known non-secret
// settings. It contains effective values, not raw YAML: URLs are normalized,
// api_base_url may be derived from base_url, and missing fields use defaults.
type ProfileView struct {
	BaseURL          string
	APIBaseURL       string
	Locale           string
	DefaultOutput    string
	ProjectListLimit int
}

// ResolveProfileView returns the display view and whether the profile is
// persisted in config.yml. Missing profiles resolve to built-in defaults. A
// persisted profile with an invalid known value returns the same typed config
// error that runtime resolution would return.
func ResolveProfileView(file *File, name string) (ProfileView, bool, error) {
	r, err := resolveProfile(file, name)
	if err != nil {
		return ProfileView{}, false, err
	}
	locale := ""
	if r.localeSet {
		locale = r.locale
	}
	return ProfileView{
		BaseURL:          r.baseURL,
		APIBaseURL:       r.apiBaseURL,
		Locale:           locale,
		DefaultOutput:    r.defaultOutput,
		ProjectListLimit: r.projectListLimit,
	}, r.persisted, nil
}

// ValidateProfiles validates every persisted profile name and known value
// without inspecting current_profile. Commands that already resolved an
// explicit selection use this to fail atomically on invalid sibling profiles
// while preserving the documented ability to bypass a bad stored selection.
func ValidateProfiles(file *File) error {
	for _, name := range sortedProfileNames(file) {
		if _, err := profileValues(file, name); err != nil {
			return err
		}
	}
	return nil
}

// ConfigValue is one known non-secret config entry for inspection output. Value
// is intentionally JSON-native: strings for text fields, ints for numeric
// fields, and nil for an unset locale.
type ConfigValue struct {
	Key    string
	Value  any
	Source string
}

// CurrentProfileValue returns current_profile with its source after validating
// the persisted identifier.
func CurrentProfileValue(file *File) (ConfigValue, error) {
	if file != nil && file.CurrentProfile != "" {
		if err := validateProfileName(file.CurrentProfile); err != nil {
			return ConfigValue{}, err
		}
		return ConfigValue{Key: "current_profile", Value: file.CurrentProfile, Source: sourceFileLabel}, nil
	}
	return ConfigValue{Key: "current_profile", Value: DefaultProfile, Source: sourceDefaultLabel}, nil
}

// ListValues returns current_profile plus every persisted profile's known
// values. When no profile is persisted it emits the built-in local profile.
// Listing is strict: one invalid persisted profile fails the whole list instead
// of emitting a partial table that could look safe to scripts.
func ListValues(file *File) ([]ConfigValue, error) {
	current, err := CurrentProfileValue(file)
	if err != nil {
		return nil, err
	}
	out := []ConfigValue{current}
	names := sortedProfileNames(file)
	if len(names) == 0 {
		names = []string{DefaultProfile}
	}
	for _, name := range names {
		values, err := profileValues(file, name)
		if err != nil {
			return nil, err
		}
		out = append(out, values...)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	})
	return out, nil
}

// GetValue returns one known non-secret config value by key. It validates all
// persisted profiles before returning a field so a direct lookup cannot make
// an otherwise invalid config appear safe when ListValues would reject it.
func GetValue(file *File, key string) (ConfigValue, error) {
	if key == "current_profile" {
		values, err := ListValues(file)
		if err != nil {
			return ConfigValue{}, err
		}
		return findConfigValue(values, key)
	}
	name, _, ok := ParseProfileKey(key)
	if !ok {
		return ConfigValue{}, newError(ErrUnsupportedKey, "", "", key, "is not a known config key", nil)
	}
	if file == nil {
		return ConfigValue{}, newError(ErrMissingProfile, "", name, "profile", "is not present in config file", nil)
	}
	if _, persisted := file.Profiles[name]; !persisted {
		return ConfigValue{}, newError(ErrMissingProfile, "", name, "profile", "is not present in config file", nil)
	}
	values, err := ListValues(file)
	if err != nil {
		return ConfigValue{}, err
	}
	return findConfigValue(values, key)
}

func findConfigValue(values []ConfigValue, key string) (ConfigValue, error) {
	for _, value := range values {
		if value.Key == key {
			return value, nil
		}
	}
	return ConfigValue{}, newError(ErrUnsupportedKey, "", "", key, "is not a known config key", nil)
}

// ParseProfileKey splits a known profiles.<name>.<suffix> key from the right so
// profile names may contain dots.
func ParseProfileKey(key string) (name, suffix string, ok bool) {
	const prefix = "profiles."
	if !strings.HasPrefix(key, prefix) {
		return "", "", false
	}
	rest := key[len(prefix):]
	for _, candidate := range []string{
		"defaults.project_list_limit",
		"api_base_url",
		"default_output",
		"base_url",
		"locale",
	} {
		suffixWithDot := "." + candidate
		if strings.HasSuffix(rest, suffixWithDot) {
			name := strings.TrimSuffix(rest, suffixWithDot)
			if name == "" {
				return "", "", false
			}
			return name, candidate, true
		}
	}
	return "", "", false
}

// profileResolved carries one profile's effective values plus their source
// labels. Keeping value and source together prevents config get/list from
// drifting apart when defaults or derived fields change.
type profileResolved struct {
	persisted              bool
	baseURL                string
	baseURLSource          string
	apiBaseURL             string
	apiBaseURLSource       string
	locale                 string
	localeSet              bool
	localeSource           string
	defaultOutput          string
	defaultOutputSource    string
	projectListLimit       int
	projectListLimitSource string
}

// resolveProfile builds effective inspection values and source labels while
// validating every known field. Name validation happens here rather than
// during YAML loading so commands can still load and repair a bad stored
// selection without allowing inspection to emit it.
func resolveProfile(file *File, name string) (profileResolved, error) {
	if err := validateProfileName(name); err != nil {
		return profileResolved{}, err
	}
	var profile Profile
	var meta profileMetadata
	ok := false
	if file != nil {
		profile, ok = file.Profiles[name]
		meta = file.profileMeta[name]
	}

	r := profileResolved{persisted: ok}
	r.baseURL = DefaultBaseURL
	r.baseURLSource = sourceDefaultLabel
	if ok || (file != nil && file.legacyAuthProfiles[name]) {
		r.baseURL = LegacyImplicitBaseURL
	}
	if ok && meta.BaseURLSet {
		r.baseURL = profile.BaseURL
		r.baseURLSource = sourceFileLabel
	}
	// Inspection uses the same effective values as runtime resolution. That
	// means a loaded but invalid known value fails before any command can print
	// it as usable JSON.
	baseURL, err := normalizeURL("base_url", r.baseURL)
	if err != nil {
		return profileResolved{}, withConfigContext(err, "", name)
	}
	r.baseURL = baseURL

	if ok && meta.APIBaseURLSet {
		r.apiBaseURL = profile.APIBaseURL
		r.apiBaseURLSource = sourceFileLabel
	} else {
		r.apiBaseURL = deriveAPIBaseURL(r.baseURL)
		r.apiBaseURLSource = sourceDerivedLabel
	}
	apiBaseURL, err := normalizeURL("api_base_url", r.apiBaseURL)
	if err != nil {
		return profileResolved{}, withConfigContext(err, "", name)
	}
	r.apiBaseURL = apiBaseURL

	r.localeSource = sourceDefaultLabel
	if ok && meta.LocaleSet {
		r.locale = profile.Locale
		r.localeSet = true
		r.localeSource = sourceFileLabel
	}
	if err := validateLocale(r.locale); err != nil {
		return profileResolved{}, withConfigContext(err, "", name)
	}

	r.defaultOutput = DefaultOutput
	r.defaultOutputSource = sourceDefaultLabel
	if ok && meta.DefaultOutputSet {
		r.defaultOutput = profile.DefaultOutput
		r.defaultOutputSource = sourceFileLabel
	}
	if err := validateDefaultOutput(r.defaultOutput); err != nil {
		return profileResolved{}, withConfigContext(err, "", name)
	}

	r.projectListLimit = DefaultProjectListLimit
	r.projectListLimitSource = sourceDefaultLabel
	if ok && meta.ProjectListLimitSet {
		r.projectListLimit = profile.Defaults.ProjectListLimit
		r.projectListLimitSource = sourceFileLabel
	}
	if err := validateProjectListLimit(r.projectListLimit); err != nil {
		return profileResolved{}, withConfigContext(err, "", name)
	}

	return r, nil
}

// profileValues expands one validated profile into the known dotted keys used
// by both config get and config list.
func profileValues(file *File, name string) ([]ConfigValue, error) {
	r, err := resolveProfile(file, name)
	if err != nil {
		return nil, err
	}
	// A missing or explicitly empty locale is rendered as JSON null / an empty
	// plain cell, matching the "no Accept-Language" runtime behavior.
	var locale any
	if r.localeSet && r.locale != "" {
		locale = r.locale
	}
	prefix := "profiles." + name + "."
	return []ConfigValue{
		{Key: prefix + "api_base_url", Value: r.apiBaseURL, Source: r.apiBaseURLSource},
		{Key: prefix + "base_url", Value: r.baseURL, Source: r.baseURLSource},
		{Key: prefix + "default_output", Value: r.defaultOutput, Source: r.defaultOutputSource},
		{Key: prefix + "defaults.project_list_limit", Value: r.projectListLimit, Source: r.projectListLimitSource},
		{Key: prefix + "locale", Value: locale, Source: r.localeSource},
	}, nil
}

func sortedProfileNames(file *File) []string {
	if file == nil {
		return nil
	}
	names := make([]string, 0, len(file.Profiles))
	for name := range file.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
