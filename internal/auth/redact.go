package auth

import "github.com/vincentsch/chab-cli/internal/redact"

// MaskAPIKey keeps the non-secret id segment of id|secret keys and hides the
// secret segment. Values without an id separator are fully redacted.
func MaskAPIKey(raw string) string {
	return redact.APIKey(raw)
}

// RedactString removes secret-shaped values from text before it is returned to
// callers or displayed in diagnostics.
func RedactString(s string) string {
	return redact.String(s)
}

// RedactBytes redacts secret-shaped byte content and returns a new slice.
func RedactBytes(b []byte) []byte {
	return redact.Bytes(b)
}

// RedactHeader redacts HTTP header values, with special handling for
// Authorization.
func RedactHeader(name, value string) string {
	return redact.Header(name, value)
}
