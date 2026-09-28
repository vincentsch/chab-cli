package config

import (
	"fmt"
	"strings"

	"github.com/vincentsch/chab-cli/internal/redact"
)

// ErrorKind is a stable machine-readable local config/auth error category.
type ErrorKind string

const (
	ErrInvalidProfileName ErrorKind = "invalid_profile_name"
	ErrInvalidURL         ErrorKind = "invalid_url"
	ErrInvalidLocale      ErrorKind = "invalid_locale"
	ErrInvalidDefault     ErrorKind = "invalid_default"
	ErrMalformedConfig    ErrorKind = "malformed_config"
	ErrUnsupportedVersion ErrorKind = "unsupported_version"
	ErrMissingProfile     ErrorKind = "missing_profile"
	ErrMissingCredential  ErrorKind = "missing_credential"
	ErrUnsupportedKey     ErrorKind = "unsupported_key"
)

// Error is returned for local config and validation failures. It is structural
// so internal/cli can classify it without importing this package.
type Error struct {
	Kind    ErrorKind
	Path    string
	Profile string
	Field   string
	Detail  string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}

	// Render-time redaction is kept even though newError also redacts. Some
	// callers add path/profile context after construction.
	var b strings.Builder
	b.WriteString("config ")
	b.WriteString(string(e.Kind))
	if e.Field != "" {
		b.WriteString(" for ")
		b.WriteString(redact.String(e.Field))
	}
	if e.Profile != "" {
		b.WriteString(" in profile ")
		b.WriteString(strconvQuote(redact.String(e.Profile)))
	}
	if e.Path != "" {
		b.WriteString(" at ")
		b.WriteString(strconvQuote(redact.String(e.Path)))
	}
	if e.Detail != "" {
		b.WriteString(": ")
		b.WriteString(redact.String(e.Detail))
	}
	if e.Err != nil {
		if e.Detail == "" {
			b.WriteString(": ")
		} else {
			b.WriteString(": ")
		}
		b.WriteString(redact.String(e.Err.Error()))
	}
	return b.String()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *Error) ExitCode() int {
	return 1
}

func newError(kind ErrorKind, path, profile, field, detail string, err error) *Error {
	return &Error{
		Kind:    kind,
		Path:    redact.String(path),
		Profile: redact.String(profile),
		Field:   redact.String(field),
		Detail:  redact.String(detail),
		Err:     err,
	}
}

func strconvQuote(s string) string {
	return fmt.Sprintf("%q", s)
}
