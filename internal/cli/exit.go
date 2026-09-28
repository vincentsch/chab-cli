package cli

import "errors"

const (
	ExitSuccess     = 0 // success
	ExitUsage       = 1 // usage, validation, config, or local preflight error
	ExitAPI         = 2 // API or network error
	ExitAuth        = 3 // authentication failure
	ExitForbidden   = 4 // authorization, plan, or project-scope denial
	ExitNotFound    = 5 // not found
	ExitRateLimited = 6 // rate limited
)

// exitCoder is intentionally structural. Domain packages can return errors
// with ExitCode without importing this CLI package.
type exitCoder interface {
	ExitCode() int
}

// ExitCodeFor classifies err into the process exit categories from the CLI
// output contract.
func ExitCodeFor(err error) int {
	if err == nil {
		return ExitSuccess
	}

	var coder exitCoder
	// Keep this structural so config/auth/API packages can opt in without
	// importing the CLI package.
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}

	return ExitUsage
}
