package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type idempotencyMode int

const (
	idempotencyNone idempotencyMode = iota
	idempotencyAuto
	idempotencyExplicit
)

// Idempotency selects idempotency-key behavior for one unsafe request. The
// zero value sends no key.
type Idempotency struct {
	mode          idempotencyMode
	key           string
	singleAttempt bool
}

// IdempotencyNone sends no idempotency key.
var IdempotencyNone Idempotency

// AutoIdempotency generates a chab-prefixed UUIDv4 idempotency key.
func AutoIdempotency() Idempotency {
	return Idempotency{mode: idempotencyAuto}
}

// GenerateIdempotencyKey returns a validated Chab idempotency key for callers
// that must persist the key before the first effectful request.
func GenerateIdempotencyKey() (string, error) {
	return generateIdempotencyKey()
}

// ExplicitIdempotency sends key after local contract validation.
func ExplicitIdempotency(key string) Idempotency {
	return Idempotency{mode: idempotencyExplicit, key: key}
}

// JournaledIdempotency requests exactly one physical attempt when replaying
// a previously unknown local action. First submissions retain normal policy.
func JournaledIdempotency(key string, priorUnknown bool) Idempotency {
	return Idempotency{mode: idempotencyExplicit, key: key, singleAttempt: priorUnknown}
}

// resolveIdempotency returns the concrete key for a request before the retry
// loop starts. That keeps all attempts for one unsafe request tied to one key.
func resolveIdempotency(idem Idempotency) (string, error) {
	switch idem.mode {
	case idempotencyNone:
		return "", nil
	case idempotencyExplicit:
		if err := validateIdempotencyKey(idem.key); err != nil {
			return "", err
		}
		return idem.key, nil
	case idempotencyAuto:
		key, err := generateIdempotencyKey()
		if err != nil {
			return "", &UsageError{Field: "idempotency-key", Detail: "could not generate idempotency key", Err: err}
		}
		return key, nil
	default:
		return "", &UsageError{Field: "idempotency-key", Detail: "unknown idempotency mode"}
	}
}

// resolveRequestIdempotency converts the caller's idempotency selection into the
// concrete header value for this request. Safe methods intentionally skip
// validation and generation, so an accidental raw GET with an idempotency option
// still behaves like an ordinary safe request.
func resolveRequestIdempotency(method string, idem Idempotency) (string, error) {
	if isSafeMethod(method) {
		return "", nil
	}
	return resolveIdempotency(idem)
}

// isSafeMethod identifies HTTP methods that do not change server state and
// therefore never need an idempotency key.
func isSafeMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// ValidateIdempotencyKey enforces the explicit idempotency-key contract:
// 1-255 bytes, every byte visible ASCII (0x21-0x7e, no spaces or control
// bytes). Commands call this in preflight so an invalid explicit key fails
// locally before credentials are resolved.
func ValidateIdempotencyKey(key string) error {
	if len(key) == 0 {
		return &UsageError{Field: "idempotency-key", Detail: "must not be empty"}
	}
	if len(key) > 255 {
		return &UsageError{Field: "idempotency-key", Detail: "must be 255 bytes or fewer"}
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x21 || key[i] > 0x7e {
			return &UsageError{Field: "idempotency-key", Detail: "must contain only visible ASCII characters without spaces"}
		}
	}
	return nil
}

func validateIdempotencyKey(key string) error {
	return ValidateIdempotencyKey(key)
}

// generateIdempotencyKey builds a UUIDv4-compatible key without adding an
// external dependency.
func generateIdempotencyKey() (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	// Set UUID version 4 and RFC 4122 variant bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	encoded := make([]byte, 32)
	hex.Encode(encoded, b[:])
	return fmt.Sprintf("chab-%s-%s-%s-%s-%s", encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32]), nil
}
