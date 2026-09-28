// Package testutil contains the shared command-test harness for chab.
//
// Command tests should execute the real CLI path with RunCommand or
// RunCommandWith and assert results with AssertSuccess and AssertExitCode.
// RunCommandWith mirrors cli.Options: tests can inject stdin and a hermetic
// LookupEnv without mutating process-global environment.
//
// BuildTree constructs a command tree and per-tree catalog without executing
// or panicking. SpecByPath, RunHelp, GenerateManualDocs, SafetyViolations, and
// AssertSafe cover help and catalog checks.
//
// HermeticEnv is the preferred LookupEnv for command tests; it resolves only
// caller-supplied values and never consults the process environment.
package testutil
