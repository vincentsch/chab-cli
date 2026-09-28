package config

import (
	"net/url"
	"regexp"
	"strings"
)

var profileNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

func validateProfileName(name string) error {
	if !profileNamePattern.MatchString(name) {
		return newError(ErrInvalidProfileName, "", name, "profile", "must match ^[a-z0-9][a-z0-9._-]{0,62}$", nil)
	}
	return nil
}

func normalizeURL(field, raw string) (string, error) {
	// Runtime endpoints must be absolute origins or base paths. Userinfo,
	// queries, and fragments are rejected so later request construction is
	// deterministic and cannot hide credentials in the URL.
	if strings.TrimSpace(raw) == "" {
		return "", newError(ErrInvalidURL, "", "", field, "must be a non-empty absolute http or https URL", nil)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", newError(ErrInvalidURL, "", "", field, "must be a valid URL", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", newError(ErrInvalidURL, "", "", field, "scheme must be http or https", nil)
	}
	if parsed.Host == "" {
		return "", newError(ErrInvalidURL, "", "", field, "must include a host", nil)
	}
	if parsed.User != nil {
		return "", newError(ErrInvalidURL, "", "", field, "must not include userinfo", nil)
	}
	// url.Parse represents "https://host?" as ForceQuery with an empty
	// RawQuery; reject that form too so all query markers are forbidden.
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return "", newError(ErrInvalidURL, "", "", field, "must not include a query string", nil)
	}
	// A bare trailing "#" parses with an empty Fragment and would otherwise be
	// normalized away, so check the raw input as well as parsed.Fragment.
	if parsed.Fragment != "" || strings.Contains(raw, "#") {
		return "", newError(ErrInvalidURL, "", "", field, "must not include a fragment", nil)
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func deriveAPIBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == DefaultBaseURL {
		return DefaultAPIBaseURL
	}
	// Older installs persisted only the app origin while the API lived on a
	// separate host. Never silently send their stored bearer to app.chab.ai.
	if baseURL == "https://app.chab.ai" {
		return "https://api.chab.ai/v1"
	}
	return baseURL + "/v1"
}

// DeriveAPIBaseURL returns the API base URL derived from a product base URL,
// matching runtime resolution's derivation.
func DeriveAPIBaseURL(baseURL string) string {
	return deriveAPIBaseURL(baseURL)
}

// ValidateEndpointURL validates a product or API endpoint URL using the same
// URL rules as runtime resolution.
func ValidateEndpointURL(raw string) error {
	_, err := normalizeURL("url", raw)
	return err
}

func validateLocale(locale string) error {
	if locale == "" || locale == "en" || locale == "de" {
		return nil
	}
	return newError(ErrInvalidLocale, "", "", "locale", "must be en or de", nil)
}

func validateDefaultOutput(value string) error {
	if value == DefaultOutput {
		return nil
	}
	return newError(ErrInvalidDefault, "", "", "default_output", "must be table", nil)
}

func validateProjectListLimit(value int) error {
	if value > 0 {
		return nil
	}
	return newError(ErrInvalidDefault, "", "", "defaults.project_list_limit", "must be a positive integer", nil)
}
