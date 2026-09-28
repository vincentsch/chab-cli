//go:build !windows

package cli_test

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestLoginOverwritePermissionWarningOnce(t *testing.T) {
	state := testutil.NewState(t)
	oldKey := testutil.FakeKey("old_warning")
	newKey := testutil.FakeKey("new_warning")
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(oldKey))
	if err := os.Chmod(state.AuthPath, 0o644); err != nil {
		t.Fatalf("chmod auth file: %v", err)
	}
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		requireWhoamiRequest(t, r)
		fmt.Fprint(w, whoamiEnvelope())
	})

	result := testutil.RunCommandWith(t, testutil.Options{
		Stdin:           strings.NewReader("y\n" + newKey + "\n"),
		StdinIsTerminal: func() bool { return true },
		Now:             func() time.Time { return commandNow },
	}, state.Args("--profile", "local", "--base-url", server.URL, "login", "--api-key")...)
	if result.ExitCode != cli.ExitSuccess {
		t.Fatalf("login result = %#v", result)
	}
	if got := strings.Count(result.Stderr, "Warning: auth file"); got != 1 {
		t.Fatalf("warning count = %d, want 1:\n%s", got, result.Stderr)
	}
	server.AssertBearer(t, newKey)
	assertStoredAuthKey(t, state.AuthPath, "local", newKey)
}

func TestLoginEarlyAuthLoadErrorWarnsPermissionOnce(t *testing.T) {
	state := testutil.NewState(t)
	testutil.WriteMalformedAuth(t, state.AuthPath)
	if err := os.Chmod(state.AuthPath, 0o644); err != nil {
		t.Fatalf("chmod auth file: %v", err)
	}
	server := testutil.NewAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	result := testutil.RunCommandWith(t, testutil.Options{
		Stdin:           strings.NewReader(testutil.FakeKey("unused_warning") + "\n"),
		StdinIsTerminal: func() bool { return true },
	}, state.Args("--profile", "local", "--base-url", server.URL, "auth", "login", "--api-key")...)
	if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
		t.Fatalf("login malformed auth result = %#v", result)
	}
	if got := strings.Count(result.Stderr, "Warning: auth file"); got != 1 {
		t.Fatalf("warning count = %d, want 1:\n%s", got, result.Stderr)
	}
	server.AssertNoRequests(t)
}
