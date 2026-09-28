package api

import (
	"context"
	"crypto/x509"
	"errors"
	"math/rand/v2"
	"net"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

const (
	defaultMaxAttempts = 3
	sleepBudget        = 30 * time.Second
	backoffBase        = 500 * time.Millisecond
	backoffCap         = 4 * time.Second
)

type attemptOutcome struct {
	status             int
	retryAfter         RetryAfter
	idempotencyOutcome string
	err                error
}

// retryDecision carries both the action and the debug reason. refusedReason is
// set when an otherwise retryable outcome is stopped by policy.
type retryDecision struct {
	retry         bool
	wait          time.Duration
	reason        string
	refusedReason string
}

// retryDecision implements the public retry contract. GET uses bounded
// jittered backoff for retryable transport and 5xx failures; any method may
// retry 429 only when the server gives a usable Retry-After wait.
func (c *Client) retryDecision(method string, outcome attemptOutcome, attempt int, remaining time.Duration) retryDecision {
	candidate := retryDecision{}
	needsJitter := false

	if outcome.err != nil {
		if strings.EqualFold(method, http.MethodGet) && isRetryableTransportError(outcome.err) {
			candidate = retryDecision{retry: true, wait: backoffWait(attempt), reason: "transport"}
			needsJitter = true
		} else {
			return retryDecision{}
		}
	} else if outcome.status == http.StatusTooManyRequests {
		// A released or settled write already crossed the idempotency boundary.
		// A retry here could start a new operation before the action journal can
		// classify the denial. Treat any marker value conservatively.
		if outcome.idempotencyOutcome != "" {
			return retryDecision{refusedReason: "idempotency_outcome"}
		}
		if outcome.retryAfter.Wait == nil {
			return retryDecision{refusedReason: "missing_or_malformed_retry_after"}
		}
		// Server-provided waits are used exactly; adding jitter would violate
		// the API's rate-limit instruction and make scripts less predictable.
		candidate = retryDecision{retry: true, wait: *outcome.retryAfter.Wait, reason: "retry_after"}
	} else if strings.EqualFold(method, http.MethodGet) && isRetryableStatus(outcome.status) {
		candidate = retryDecision{retry: true, wait: backoffWait(attempt), reason: "status_5xx"}
		needsJitter = true
	} else {
		return retryDecision{}
	}

	if attempt >= c.maxAttempts {
		return retryDecision{refusedReason: "attempt_cap"}
	}
	if needsJitter {
		candidate.wait = c.backoffJitter(candidate.wait)
		if candidate.wait < 0 {
			candidate.wait = 0
		}
	}
	if candidate.wait > remaining {
		return retryDecision{refusedReason: "budget"}
	}
	return candidate
}

// backoffWait returns the deterministic pre-jitter wait before retry n
// (1-based): 500ms, 1s, 2s, 4s, 4s, ...
func backoffWait(retry int) time.Duration {
	if retry <= 1 {
		return backoffBase
	}
	wait := backoffBase
	for i := 1; i < retry; i++ {
		wait *= 2
		if wait >= backoffCap {
			return backoffCap
		}
	}
	return wait
}

func isRetryableStatus(status int) bool {
	return status == http.StatusInternalServerError ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
}

// isRetryableTransportError is intentionally conservative. Caller-initiated
// cancellation, caller deadlines, and certificate trust failures are final;
// transient network failures may be retried for GET by retryDecision.
func isRetryableTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var certUnknown x509.UnknownAuthorityError
	var certInvalid x509.CertificateInvalidError
	var certHostname x509.HostnameError
	if errors.As(err, &certUnknown) || errors.As(err, &certInvalid) || errors.As(err, &certHostname) {
		return false
	}

	var urlErr *neturl.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		// net/http wraps most client errors in *url.Error. Classify the
		// underlying error so context and net.Error checks behave as intended.
		err = urlErr.Err
	}

	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		op := strings.ToLower(opErr.Op)
		if op == "dial" || op == "connect" {
			return true
		}
		lower := strings.ToLower(opErr.Error())
		return strings.Contains(lower, "connection refused") ||
			strings.Contains(lower, "connection reset") ||
			strings.Contains(lower, "connection aborted")
	}

	return false
}

// randomBackoffJitter adds a small positive jitter so concurrent clients do
// not retry at exactly the same instant.
func randomBackoffJitter(wait time.Duration) time.Duration {
	if wait <= 0 {
		return wait
	}
	jitterMax := wait / 10
	if jitterMax <= 0 {
		return wait
	}
	return wait + time.Duration(rand.Int64N(int64(jitterMax)+1))
}

// realSleeper is the production Sleeper. Tests inject fakes so retry tests do
// not wait for wall-clock time.
type realSleeper struct{}

// RealSleeper exposes the production sleeper for command flows that own their
// own retry or polling loops.
func RealSleeper() Sleeper {
	return realSleeper{}
}

func (realSleeper) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
