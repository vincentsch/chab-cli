package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const (
	mainHelperEnvKey        = "GO_WANT_CHAB_MAIN_HELPER_PROCESS"
	mainHelperEnvValue      = "classified-exit"
	mainHelperFailureMarker = "main helper returned without exiting"
	mainHelperFailureExit   = 99
)

// TestMainUsesClassifiedExitCode covers the process boundary that in-process
// command tests cannot: main must turn cli.Run's classified code into the real
// process exit status.
func TestMainUsesClassifiedExitCode(t *testing.T) {
	if os.Getenv(mainHelperEnvKey) == mainHelperEnvValue {
		os.Args = []string{"chab", "help", "bogus"}
		main()
		// main should always terminate the child with os.Exit. Use a sentinel
		// status distinct from the expected usage exit if that contract breaks.
		_, _ = os.Stderr.WriteString(mainHelperFailureMarker + "\n")
		os.Exit(mainHelperFailureExit)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainUsesClassifiedExitCode$")
	// "help bogus" is resolved from the command tree before config or auth
	// lookup, so inheriting the parent environment cannot affect this proof.
	cmd.Env = append(os.Environ(), mainHelperEnvKey+"="+mainHelperEnvValue)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatalf("helper exited successfully; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("helper failed without process exit status: %v", err)
	}
	if got := exitErr.ExitCode(); got != 1 {
		t.Fatalf("exit status = %d, want 1; stdout=%q stderr=%q", got, stdout.String(), stderr.String())
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}

	gotStderr := stderr.String()
	if !strings.Contains(gotStderr, "unknown command") {
		t.Fatalf("stderr missing unknown command message: %q", gotStderr)
	}
	if strings.Contains(gotStderr, mainHelperFailureMarker) {
		t.Fatalf("stderr contains helper failure marker: %q", gotStderr)
	}
}
