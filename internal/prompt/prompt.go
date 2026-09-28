// Package prompt contains minimal interactive input helpers for command code.
package prompt

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// Default selects the answer used for a blank confirmation line.
type Default int

const (
	DefaultNo  Default = iota // a blank answer denies
	DefaultYes                // a blank answer accepts
)

// Prompter reads interactive input. Prompt text is written to Err so stdout
// stays reserved for command output.
type Prompter struct {
	in           io.Reader
	err          io.Writer
	inIsTerminal bool
}

// New creates a Prompter over injected input and prompt output streams.
func New(in io.Reader, err io.Writer, inIsTerminal bool) *Prompter {
	if in == nil {
		in = bytes.NewReader(nil)
	}
	if err == nil {
		err = io.Discard
	}
	return &Prompter{in: in, err: err, inIsTerminal: inIsTerminal}
}

// InteractiveIn reports whether the input behaves like a terminal.
func (p *Prompter) InteractiveIn() bool {
	return p != nil && p.inIsTerminal
}

// Text prompts as "<label> [<prefill>]: " and returns the entered line, or
// prefill when the line is empty.
func (p *Prompter) Text(label, prefill string) (string, error) {
	return p.TextWithDisplay(label, prefill, prefill)
}

// TextWithDisplay prompts with a separately prepared display value while
// retaining the real prefill as the value returned for blank input.
func (p *Prompter) TextWithDisplay(label, prefill, displayPrefill string) (string, error) {
	var writeErr error
	if displayPrefill == "" {
		_, writeErr = fmt.Fprintf(p.err, "%s: ", label)
	} else {
		_, writeErr = fmt.Fprintf(p.err, "%s [%s]: ", label, displayPrefill)
	}
	if writeErr != nil {
		return "", fmt.Errorf("write text prompt: %w", writeErr)
	}
	value, err := p.readLine()
	if err != nil {
		return "", err
	}
	if value == "" {
		return prefill, nil
	}
	return value, nil
}

// Confirm asks a yes/no question and returns whether the user accepted.
// Answers are case-insensitive: y/yes accept, n/no deny. A blank line uses
// def. Unrecognized input re-prompts. EOF is denial, reported as (false, nil).
func (p *Prompter) Confirm(question string, def Default) (bool, error) {
	suffix := " [y/N]: "
	if def == DefaultYes {
		suffix = " [Y/n]: "
	}
	for {
		if _, err := fmt.Fprintf(p.err, "%s%s", question, suffix); err != nil {
			return false, fmt.Errorf("write confirmation prompt: %w", err)
		}
		line, err := p.readLine()
		if err != nil {
			if err == io.EOF {
				return false, nil
			}
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		case "":
			return def == DefaultYes, nil
		}
	}
}

// Select renders a numbered menu and returns the selected zero-based index and
// value. EOF is returned as a read error.
func (p *Prompter) Select(label string, options []string) (int, string, error) {
	if len(options) == 0 {
		return 0, "", fmt.Errorf("select: no options provided")
	}
	for {
		fmt.Fprintf(p.err, "%s\n", label)
		for i, opt := range options {
			fmt.Fprintf(p.err, "  %d) %s\n", i+1, opt)
		}
		fmt.Fprintf(p.err, "Selection [1-%d]: ", len(options))
		line, err := p.readLine()
		if err != nil {
			return 0, "", err
		}
		choice, convErr := strconv.Atoi(strings.TrimSpace(line))
		if convErr != nil || choice < 1 || choice > len(options) {
			continue
		}
		return choice - 1, options[choice-1], nil
	}
}

// Secret prompts for a secret value without echo when the input terminal
// supports it, falling back to a plain line read for injected or unsupported
// readers.
func (p *Prompter) Secret(label string) (string, error) {
	fmt.Fprintf(p.err, "%s: ", label)
	if p.inIsTerminal {
		if fd, ok := p.in.(interface{ Fd() uintptr }); ok {
			value, err := term.ReadPassword(int(fd.Fd()))
			if err == nil {
				fmt.Fprintln(p.err)
				return trimCR(string(value)), nil
			}
		}
	}
	return p.readLine()
}

// ReadPiped consumes the whole input, for non-TTY API key input.
func (p *Prompter) ReadPiped() (string, error) {
	data, err := io.ReadAll(p.in)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// IsTerminalReader reports whether r exposes a file descriptor that is a
// terminal. Buffers and pipes report false.
func IsTerminalReader(r io.Reader) bool {
	fd, ok := r.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(fd.Fd()))
}

// IsTerminalWriter reports whether w exposes a file descriptor that is a
// terminal. Buffers and pipes report false.
func IsTerminalWriter(w io.Writer) bool {
	fd, ok := w.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(fd.Fd()))
}

// TerminalHeightWriter reports terminal rows for w when available.
func TerminalHeightWriter(w io.Writer) (int, bool) {
	fd, ok := w.(interface{ Fd() uintptr })
	if !ok {
		return 0, false
	}
	_, height, err := term.GetSize(int(fd.Fd()))
	if err != nil || height <= 0 {
		return 0, false
	}
	return height, true
}

func (p *Prompter) readLine() (string, error) {
	// Read one byte at a time so tests can use simple readers and so CRLF input
	// from Windows terminals normalizes the same way as LF input.
	var buf []byte
	var one [1]byte
	for {
		n, err := p.in.Read(one[:])
		if n > 0 {
			if one[0] == '\n' {
				return trimCR(string(buf)), nil
			}
			buf = append(buf, one[0])
		}
		if err != nil {
			if err == io.EOF && len(buf) > 0 {
				return trimCR(string(buf)), nil
			}
			return "", err
		}
	}
}

func trimCR(value string) string {
	if len(value) > 0 && value[len(value)-1] == '\r' {
		return value[:len(value)-1]
	}
	return value
}
