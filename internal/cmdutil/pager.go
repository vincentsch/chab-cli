package cmdutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
)

const unknownHeightPagerThreshold = 40

var pagerRuntimeGOOS = runtime.GOOS

// PagerRunner executes a parsed pager argv with fully buffered human output.
// It must not invoke a shell.
type PagerRunner func(ctx context.Context, argv []string, input []byte, stdout, stderr io.Writer) error

// PagerStartError marks pager launch failures that should fall back to direct
// stdout. A pager that starts and exits nonzero should return PagerExitError.
type PagerStartError struct {
	Err error
}

func (e *PagerStartError) Error() string {
	if e == nil || e.Err == nil {
		return "could not start pager"
	}
	return "could not start pager: " + e.Err.Error()
}

func (e *PagerStartError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// PagerExitError is returned when a pager process starts but exits nonzero.
type PagerExitError struct {
	Err error
}

func (e *PagerExitError) Error() string {
	if e == nil || e.Err == nil {
		return "pager failed"
	}
	return "pager failed: " + e.Err.Error()
}

func (e *PagerExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *PagerExitError) ExitCode() int {
	return 1
}

// DefaultPagerRunner runs the pager directly, wiring the rendered output to
// stdin. It distinguishes launch failures from nonzero pager exits so dispatch
// can fall back only when the pager never started.
func DefaultPagerRunner(ctx context.Context, argv []string, input []byte, stdout, stderr io.Writer) error {
	if len(argv) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &PagerExitError{Err: err}
		}
		return &PagerStartError{Err: err}
	}
	return nil
}

// maybePageHuman sends long default human output to a pager when terminal state
// and flags allow it. It falls back to direct stdout only when the pager never
// starts; a pager that starts and fails owns the already-consumed output.
func (f *Factory) maybePageHuman(ctx context.Context, stdout, stderr io.Writer, data []byte, noANSI, noPager bool) (bool, error) {
	if len(data) == 0 || noANSI || noPager || !f.stdoutIsTerminal() {
		return false, nil
	}
	height, ok := f.terminalHeight()
	threshold := unknownHeightPagerThreshold
	if ok && height > 0 {
		threshold = height
	}
	if renderedLineCount(data) <= threshold {
		return false, nil
	}
	argv, ok := f.pagerArgv()
	if !ok || len(argv) == 0 {
		return false, nil
	}
	err := f.pagerRunner()(ctx, argv, data, stdout, stderr)
	if err == nil {
		return true, nil
	}
	var startErr *PagerStartError
	if errors.As(err, &startErr) {
		return false, nil
	}
	return true, err
}

func (f *Factory) pagerArgv() ([]string, bool) {
	for _, name := range []string{"CHAB_PAGER", "PAGER"} {
		if value, ok := f.lookupEnv(name); ok && strings.TrimSpace(value) != "" {
			argv, err := parsePagerArgv(value)
			if err != nil {
				return nil, false
			}
			return argv, len(argv) > 0
		}
	}
	if pagerRuntimeGOOS == "windows" {
		return nil, false
	}
	return []string{"less", "-FRX"}, true
}

// renderedLineCount treats a final line without a trailing newline as visible.
func renderedLineCount(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	lines := bytes.Count(data, []byte{'\n'})
	if !bytes.HasSuffix(data, []byte{'\n'}) {
		lines++
	}
	return lines
}

// parsePagerArgv supports the small shell-like quoting users expect in PAGER
// without invoking a shell or honoring shell expansion.
func parsePagerArgv(input string) ([]string, error) {
	var argv []string
	var b strings.Builder
	inSingle := false
	inDouble := false
	escaped := false
	tokenActive := false

	for _, r := range input {
		switch {
		case escaped:
			b.WriteRune(r)
			tokenActive = true
			escaped = false
		case r == '\\':
			escaped = true
			tokenActive = true
		case inSingle:
			if r == '\'' {
				inSingle = false
			} else {
				b.WriteRune(r)
			}
			tokenActive = true
		case inDouble:
			if r == '"' {
				inDouble = false
			} else {
				b.WriteRune(r)
			}
			tokenActive = true
		case r == '\'':
			inSingle = true
			tokenActive = true
		case r == '"':
			inDouble = true
			tokenActive = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if tokenActive {
				argv = append(argv, b.String())
				b.Reset()
				tokenActive = false
			}
		default:
			b.WriteRune(r)
			tokenActive = true
		}
	}
	if escaped {
		return nil, fmt.Errorf("pager command ends with an escape")
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("pager command has an unterminated quote")
	}
	if tokenActive {
		argv = append(argv, b.String())
	}
	return argv, nil
}
