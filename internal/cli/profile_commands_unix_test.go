//go:build !windows

package cli_test

import (
	"os"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestProfileCommandsKeepAuthPermissionWarningsOnStderr(t *testing.T) {
	state := testutil.NewState(t)
	testutil.WriteAuthProfile(t, state.AuthPath, "local", testutil.AuthRecord(testutil.FakeKey("profile_permissions")))
	if err := os.Chmod(state.AuthPath, 0o644); err != nil {
		t.Fatal(err)
	}

	result := runLocalCommand(t, state, nil, "profile", "list", "--json")
	if result.ExitCode != cli.ExitSuccess || !strings.HasPrefix(strings.TrimSpace(result.Stdout), "{") {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Stderr, "permissions") || strings.Contains(result.Stdout, "permissions") {
		t.Fatalf("permission warning crossed output boundary: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}

	testutil.WriteMalformedAuth(t, state.AuthPath)
	if err := os.Chmod(state.AuthPath, 0o644); err != nil {
		t.Fatal(err)
	}
	failed := runLocalCommand(t, state, nil, "profile", "show", "--json")
	if failed.ExitCode != cli.ExitUsage || failed.Stdout != "" ||
		!strings.Contains(failed.Stderr, "permissions") || !strings.Contains(failed.Stderr, "could not parse auth JSON") {
		t.Fatalf("malformed broad auth result = %#v", failed)
	}

	createState := testutil.NewState(t)
	testutil.WriteAuthProfile(t, createState.AuthPath, "existing", testutil.AuthRecord(testutil.FakeKey("create_permissions")))
	if err := os.Chmod(createState.AuthPath, 0o644); err != nil {
		t.Fatal(err)
	}
	created := runLocalCommand(t, createState, nil, "profile", "create", "new", "--base-url", "https://new.example.test")
	if created.ExitCode != cli.ExitSuccess || !strings.Contains(created.Stderr, "permissions") {
		t.Fatalf("create permission result = %#v", created)
	}

	setState := testutil.NewState(t)
	testutil.WriteConfigProfile(t, setState.ConfigPath, "local", config.Profile{
		BaseURL:    "https://old.example.test",
		APIBaseURL: "https://api.example.test/v1",
	}, true)
	testutil.WriteAuthProfile(t, setState.AuthPath, "local", testutil.AuthRecord(testutil.FakeKey("set_permissions")))
	if err := os.Chmod(setState.AuthPath, 0o644); err != nil {
		t.Fatal(err)
	}
	updated := runLocalCommand(t, setState, nil, "config", "set", "profiles.local.base_url", "https://new.example.test")
	if updated.ExitCode != cli.ExitSuccess || !strings.Contains(updated.Stderr, "permissions") ||
		strings.Contains(updated.Stdout, "permissions") {
		t.Fatalf("config set permission result = %#v", updated)
	}
}
