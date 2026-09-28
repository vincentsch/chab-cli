package auth

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorKind is a stable machine-readable local auth error category.
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
)

// Error is returned for local auth and credential lookup failures. It carries
// structured context for CLI exit-code handling while redacting display text.
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
	// callers add path/profile/field context after construction.
	var b strings.Builder
	b.WriteString("auth ")
	b.WriteString(string(e.Kind))
	if e.Field != "" {
		b.WriteString(" for ")
		b.WriteString(RedactString(e.Field))
	}
	if e.Profile != "" {
		b.WriteString(" in profile ")
		b.WriteString(fmt.Sprintf("%q", RedactString(e.Profile)))
	}
	if e.Path != "" {
		b.WriteString(" at ")
		b.WriteString(fmt.Sprintf("%q", RedactString(e.Path)))
	}
	if e.Detail != "" {
		b.WriteString(": ")
		b.WriteString(RedactString(e.Detail))
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(RedactString(e.Err.Error()))
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
	if e != nil && e.Kind == ErrMissingCredential {
		return 3
	}
	return 1
}

func newError(kind ErrorKind, path, profile, field, detail string, err error) *Error {
	return &Error{
		Kind:    kind,
		Path:    RedactString(path),
		Profile: RedactString(profile),
		Field:   RedactString(field),
		Detail:  RedactString(detail),
		Err:     err,
	}
}

// withAuthContext adds boundary-specific file/profile context to an auth error
// created deeper in the write path. It returns a copy so callers that hold the
// original error do not observe context from a different boundary.
func withAuthContext(err error, path, profile string) error {
	var authErr *Error
	if !errors.As(err, &authErr) {
		return err
	}
	copyErr := *authErr
	if copyErr.Path == "" {
		copyErr.Path = RedactString(path)
	}
	if copyErr.Profile == "" {
		copyErr.Profile = RedactString(profile)
	}
	return &copyErr
}
