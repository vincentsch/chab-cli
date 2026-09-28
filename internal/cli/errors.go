package cli

import "fmt"

// ReservedCommandError is returned when a reserved leaf command is executed.
// It is a normal local usage failure, not a placeholder panic or missing route.
type ReservedCommandError struct {
	Path string
}

func (e *ReservedCommandError) Error() string {
	return fmt.Sprintf("%q is not implemented yet: this command is planned for a later release. Run %q for details.", e.Path, e.Path+" --help")
}

func (e *ReservedCommandError) ExitCode() int {
	return ExitUsage
}

// unknownSubcommandError carries enough command-path context for the shared
// renderer to print Cobra-style usage guidance.
type unknownSubcommandError struct {
	Name string
	Path string
}

func (e *unknownSubcommandError) Error() string {
	return fmt.Sprintf("unknown command %q for %q", e.Name, e.Path)
}

func (e *unknownSubcommandError) ExitCode() int {
	return ExitUsage
}

// flagParseError preserves the command path for flag parser failures so the
// shared renderer can print a useful "run --help" hint.
type flagParseError struct {
	path string
	err  error
}

func (e *flagParseError) Error() string {
	return e.err.Error()
}

func (e *flagParseError) Unwrap() error {
	return e.err
}

func (e *flagParseError) ExitCode() int {
	return ExitUsage
}
