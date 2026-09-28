package config

import "strconv"

// SetProfileBaseURL updates one persisted profile's base_url. If api_base_url
// was absent or matched the old derived value, it remains derived from the new
// base URL.
func SetProfileBaseURL(file *File, name, rawURL string) (apiBaseURL string, rederived bool, err error) {
	if file == nil {
		return "", false, newError(ErrMalformedConfig, "", name, "file", "must not be nil", nil)
	}
	ensureFileMaps(file)
	profile, ok := file.Profiles[name]
	if !ok {
		return "", false, newError(ErrMissingProfile, "", name, "profile", "is not present in config file", nil)
	}
	normalized, err := normalizeURL("base_url", rawURL)
	if err != nil {
		return "", false, withConfigContext(err, "", name)
	}

	meta := file.profileMeta[name]
	oldBaseURL := LegacyImplicitBaseURL
	if meta.BaseURLSet {
		oldBaseURL = profile.BaseURL
	}
	oldBaseURL, err = normalizeURL("base_url", oldBaseURL)
	if err != nil {
		return "", false, withConfigContext(err, "", name)
	}
	oldDerived := deriveAPIBaseURL(oldBaseURL)

	derived := !meta.APIBaseURLSet
	if meta.APIBaseURLSet {
		oldAPIBaseURL, err := normalizeURL("api_base_url", profile.APIBaseURL)
		if err != nil {
			return "", false, withConfigContext(err, "", name)
		}
		// Write materializes derived api_base_url values into YAML. Treat an
		// explicit value equal to the old derivation as still derived, so later
		// base_url edits keep following the base URL instead of freezing it.
		derived = oldAPIBaseURL == oldDerived
	}

	profile.BaseURL = normalized
	meta.BaseURLSet = true
	if derived {
		// Keep the metadata unset when the API URL is derived. Write will fill
		// the scalar for the v1 file shape, but inspection/setters can still
		// distinguish derived behavior through the metadata before the next load.
		profile.APIBaseURL = ""
		meta.APIBaseURLSet = false
		apiBaseURL = deriveAPIBaseURL(normalized)
		rederived = true
	}
	file.Profiles[name] = profile
	file.profileMeta[name] = meta
	return apiBaseURL, rederived, nil
}

// SetProfileAPIBaseURL updates one persisted profile's api_base_url.
func SetProfileAPIBaseURL(file *File, name, rawURL string) error {
	if file == nil {
		return newError(ErrMalformedConfig, "", name, "file", "must not be nil", nil)
	}
	ensureFileMaps(file)
	profile, ok := file.Profiles[name]
	if !ok {
		return newError(ErrMissingProfile, "", name, "profile", "is not present in config file", nil)
	}
	normalized, err := normalizeURL("api_base_url", rawURL)
	if err != nil {
		return withConfigContext(err, "", name)
	}
	meta := file.profileMeta[name]
	profile.APIBaseURL = normalized
	meta.APIBaseURLSet = true
	file.Profiles[name] = profile
	file.profileMeta[name] = meta
	return nil
}

// SetProfileLocale updates one persisted profile's locale. The empty string is
// valid and persists an unset Accept-Language preference.
func SetProfileLocale(file *File, name, value string) error {
	if file == nil {
		return newError(ErrMalformedConfig, "", name, "file", "must not be nil", nil)
	}
	ensureFileMaps(file)
	profile, ok := file.Profiles[name]
	if !ok {
		return newError(ErrMissingProfile, "", name, "profile", "is not present in config file", nil)
	}
	if err := validateLocale(value); err != nil {
		return withConfigContext(err, "", name)
	}
	meta := file.profileMeta[name]
	profile.Locale = value
	meta.LocaleSet = true
	file.Profiles[name] = profile
	file.profileMeta[name] = meta
	return nil
}

// SetProfileProjectListLimit updates one persisted profile's
// defaults.project_list_limit.
func SetProfileProjectListLimit(file *File, name, raw string) error {
	if file == nil {
		return newError(ErrMalformedConfig, "", name, "file", "must not be nil", nil)
	}
	ensureFileMaps(file)
	profile, ok := file.Profiles[name]
	if !ok {
		return newError(ErrMissingProfile, "", name, "profile", "is not present in config file", nil)
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return withConfigContext(newError(ErrInvalidDefault, "", name, "defaults.project_list_limit", "must be a positive integer", err), "", name)
	}
	if err := validateProjectListLimit(limit); err != nil {
		return withConfigContext(err, "", name)
	}
	meta := file.profileMeta[name]
	profile.Defaults.ProjectListLimit = limit
	meta.DefaultsSet = true
	meta.ProjectListLimitSet = true
	file.Profiles[name] = profile
	file.profileMeta[name] = meta
	return nil
}
