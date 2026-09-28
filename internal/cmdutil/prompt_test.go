package cmdutil_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
)

func TestPromptPolicyYesAndSelect(t *testing.T) {
	var stderr bytes.Buffer
	cmd := promptCommand(&stderr)
	if err := cmd.Root().PersistentFlags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	factory := &cmdutil.Factory{
		Stdin:           strings.NewReader("2\n"),
		StdinIsTerminal: func() bool { return true },
	}

	p := factory.Prompt(cmd)
	if !p.Yes() {
		t.Fatalf("Prompt.Yes() = false, want true")
	}
	index, value, err := p.Select("Choose profile", []string{"local", "staging"})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if index != 1 || value != "staging" {
		t.Fatalf("Select() = %d, %q, want 1, staging", index, value)
	}
	if !strings.Contains(stderr.String(), "Choose profile") || !strings.Contains(stderr.String(), "Selection [1-2]:") {
		t.Fatalf("selection prompt not written to stderr:\n%s", stderr.String())
	}
}

func TestConfirmDestructiveDenial(t *testing.T) {
	var stderr bytes.Buffer
	cmd := promptCommand(&stderr)
	factory := &cmdutil.Factory{
		Stdin:           strings.NewReader("\n"),
		StdinIsTerminal: func() bool { return true },
	}

	err := cmdutil.ConfirmDestructive(factory.Prompt(cmd), "Delete project?")
	var abort *cmdutil.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("ConfirmDestructive() error = %T, want AbortError", err)
	}
	if !strings.Contains(err.Error(), "no changes were made") {
		t.Fatalf("abort message = %q", err.Error())
	}
	if !strings.Contains(stderr.String(), "Delete project? [y/N]:") {
		t.Fatalf("confirmation prompt missing:\n%s", stderr.String())
	}
}

func TestConfirmDestructiveNonInteractiveAbort(t *testing.T) {
	var stderr bytes.Buffer
	cmd := promptCommand(&stderr)
	factory := &cmdutil.Factory{
		Stdin:           strings.NewReader("ignored\n"),
		StdinIsTerminal: func() bool { return false },
	}

	err := cmdutil.ConfirmDestructive(factory.Prompt(cmd), "Delete project?")
	var abort *cmdutil.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("ConfirmDestructive() error = %T, want AbortError", err)
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("abort message does not name --yes: %q", err.Error())
	}
	if stderr.String() != "" {
		t.Fatalf("non-interactive confirmation wrote prompt:\n%s", stderr.String())
	}
}

func TestConfirmDestructiveYesBypass(t *testing.T) {
	var stderr bytes.Buffer
	cmd := promptCommand(&stderr)
	if err := cmd.Root().PersistentFlags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	factory := &cmdutil.Factory{
		Stdin:           strings.NewReader(""),
		StdinIsTerminal: func() bool { return false },
	}

	if err := cmdutil.ConfirmDestructive(factory.Prompt(cmd), "Delete project?"); err != nil {
		t.Fatalf("ConfirmDestructive() error = %v", err)
	}
	if stderr.String() != "" {
		t.Fatalf("--yes bypass wrote prompt:\n%s", stderr.String())
	}
}

func TestAbortErrorExitCode(t *testing.T) {
	err := &cmdutil.AbortError{Message: "stopped"}
	if err.Error() != "stopped" || err.ExitCode() != 1 {
		t.Fatalf("AbortError = %q, code %d", err.Error(), err.ExitCode())
	}
}

func promptCommand(stderr *bytes.Buffer) *cobra.Command {
	root := &cobra.Command{Use: "root"}
	root.SetErr(stderr)
	root.PersistentFlags().Bool("no-prompt", false, "")
	root.PersistentFlags().Bool("yes", false, "")
	child := &cobra.Command{Use: "child"}
	child.SetErr(stderr)
	root.AddCommand(child)
	return child
}
