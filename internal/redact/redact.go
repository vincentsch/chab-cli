// Package redact contains deterministic secret redaction helpers shared by
// local config/auth errors and by the public auth redaction wrappers.
package redact

import (
	"regexp"
	"strings"
)

const token = "[REDACTED]"

var (
	apiKeyPairPattern = regexp.MustCompile(`(ak_[A-Za-z0-9_-]{2,}(?:\||%(?:25)*7[Cc]))([^\s"']+)`)
	apiKeyJSONPattern = regexp.MustCompile(`(?i)("api_key"\s*:\s*")([^"]*)(")`)
	authorizationLine = regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)([^\s"'\r\n]+)`)
)

// APIKey redacts key material while preserving the id segment of id|secret
// values, which is useful for support logs.
func APIKey(raw string) string {
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, "|"); idx >= 0 {
		return raw[:idx+1] + token
	}
	return token
}

// String removes secret-shaped values from text that may be returned in errors
// or diagnostics.
func String(s string) string {
	if s == "" {
		return s
	}
	s = apiKeyJSONPattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := apiKeyJSONPattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		return parts[1] + APIKey(parts[2]) + parts[3]
	})
	s = authorizationLine.ReplaceAllString(s, `${1}`+token)
	s = apiKeyPairPattern.ReplaceAllString(s, `${1}`+token)
	return s
}

// Bytes redacts a byte slice and returns a new slice.
func Bytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	return []byte(String(string(b)))
}

// Header redacts HTTP header values, with special handling for Authorization.
func Header(name, value string) string {
	if !strings.EqualFold(name, "Authorization") {
		return String(value)
	}
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= len("Bearer ") && strings.EqualFold(trimmed[:len("Bearer ")], "Bearer ") {
		return "Bearer " + token
	}
	return token
}
