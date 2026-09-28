// Package releasecheck pins the committed release smoke scripts and playbook
// contracts without requiring live credentials or a built binary.
package releasecheck

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

type scriptResult struct {
	stdout   string
	stderr   string
	exitCode int
}

// runScript runs a committed POSIX script under sh with an optional explicit
// env (nil inherits the process env) and returns its streams and exit code.
func runScript(t *testing.T, rel string, env []string, args ...string) scriptResult {
	t.Helper()
	repoRoot := findRepoRoot(t)
	full := append([]string{filepath.Join(repoRoot, filepath.FromSlash(rel))}, args...)
	cmd := exec.Command("sh", full...)
	if env != nil {
		cmd.Env = env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	exit := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exit = ee.ExitCode()
		} else {
			t.Fatalf("run %s: %v", rel, err)
		}
	}
	return scriptResult{stdout.String(), stderr.String(), exit}
}

// cleanEnv builds a minimal env with no CHAB_* values so opt-in guards trigger
// deterministically regardless of the developer's shell. PATH lets sh and the
// coreutils the scripts call resolve.
func cleanEnv(extra map[string]string) []string {
	env := []string{"PATH=" + os.Getenv("PATH")}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// skipWindows keeps script-executing tests off Windows while pure file and
// catalog assertions in this package still run there.
func skipWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("release smoke script tests use POSIX sh")
	}
}

// findRepoRoot lets this test-only package run from any package working
// directory selected by go test ./....
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root")
		}
		dir = parent
	}
}
