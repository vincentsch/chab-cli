package cli_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func TestYesIsInertOnOfflineCommands(t *testing.T) {
	rootHelp := testutil.RunCommand(t, "--help")
	yesHelp := testutil.RunCommand(t, "--yes", "--help")
	if yesHelp.ExitCode != cli.ExitSuccess || yesHelp.Stderr != "" || yesHelp.Stdout != rootHelp.Stdout {
		t.Fatalf("--yes root help result = %#v; root help = %#v", yesHelp, rootHelp)
	}

	version := testutil.RunCommand(t, "version")
	yesVersion := testutil.RunCommand(t, "version", "--yes")
	if yesVersion.ExitCode != cli.ExitSuccess || yesVersion.Stderr != "" || yesVersion.Stdout != version.Stdout {
		t.Fatalf("version --yes result = %#v; version = %#v", yesVersion, version)
	}
}

func TestUnknownCommandsAreUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"bogus"},
		{"project", "bogus"},
		{"credits", "bogus"},
		{"profile", "bogus"},
		{"config", "bogus"},
		{"completion", "nope"},
		{"help", "bogus"},
		{"help", "project", "bogus"},
		{"help", "credits", "bogus"},
		{"help", "profile", "bogus"},
		{"help", "config", "bogus"},
	} {
		result := testutil.RunCommand(t, args...)
		if result.ExitCode != cli.ExitUsage || result.Stdout != "" {
			t.Fatalf("%v result = %#v", args, result)
		}
		if !strings.Contains(result.Stderr, "unknown command") {
			t.Fatalf("%v stderr missing unknown command:\n%s", args, result.Stderr)
		}
		if len(args) > 1 && (args[0] == "credits" || args[0] == "profile" || args[0] == "config") &&
			!strings.Contains(result.Stderr, `Run "chab `+args[0]+` --help" for usage.`) {
			t.Fatalf("%v stderr missing family path guidance:\n%s", args, result.Stderr)
		}
		if len(args) > 2 && args[0] == "help" &&
			(args[1] == "credits" || args[1] == "profile" || args[1] == "config") &&
			!strings.Contains(result.Stderr, `Run "chab `+args[1]+` --help" for usage.`) {
			t.Fatalf("%v stderr missing family path guidance:\n%s", args, result.Stderr)
		}
	}
}

func TestCompletionCommands(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		result := testutil.RunCommand(t, "completion", shell)
		if result.ExitCode != cli.ExitSuccess || result.Err != nil || result.Stderr != "" {
			t.Fatalf("completion %s result = %#v", shell, result)
		}
		if !strings.Contains(result.Stdout, "chab") {
			t.Fatalf("completion %s output missing chab", shell)
		}
	}

	helpCompletion := testutil.RunCommand(t, "__complete", "help", "")
	if helpCompletion.ExitCode != cli.ExitSuccess {
		t.Fatalf("help completion result = %#v", helpCompletion)
	}
	if !strings.Contains(helpCompletion.Stdout, "health") || !strings.Contains(helpCompletion.Stdout, ":4") {
		t.Fatalf("help completion missing candidates or no-file directive:\nstdout=%s\nstderr=%s", helpCompletion.Stdout, helpCompletion.Stderr)
	}
}

func TestVersionOutput(t *testing.T) {
	human := testutil.RunCommand(t, "version")
	if human.ExitCode != cli.ExitSuccess || human.Stderr != "" {
		t.Fatalf("version result = %#v", human)
	}
	wantHuman := fmt.Sprintf("chab %s (commit %s, built %s, %s %s/%s)\n", cli.Version, cli.Commit, cli.Date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	if human.Stdout != wantHuman {
		t.Fatalf("version stdout = %q, want %q", human.Stdout, wantHuman)
	}

	origVersion, origCommit, origDate := cli.Version, cli.Commit, cli.Date
	t.Cleanup(func() {
		cli.Version, cli.Commit, cli.Date = origVersion, origCommit, origDate
	})
	cli.Version, cli.Commit, cli.Date = "1.2.3", "abc123", "2026-07-14T09:37:24Z"

	machine := testutil.RunCommand(t, "version", "--json")
	if machine.ExitCode != cli.ExitSuccess || machine.Stderr != "" {
		t.Fatalf("version --json result = %#v", machine)
	}
	if !strings.HasSuffix(machine.Stdout, "\n") || !strings.Contains(machine.Stdout, "\n  \"commit\":") {
		t.Fatalf("version JSON is not pretty printed with trailing newline:\n%s", machine.Stdout)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(machine.Stdout), &got); err != nil {
		t.Fatalf("version JSON parse error = %v\n%s", err, machine.Stdout)
	}
	wantKeys := []string{"arch", "commit", "date", "go_version", "os", "version"}
	if keys := sortedKeys(got); !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("version JSON keys = %v, want %v", keys, wantKeys)
	}
	if got["version"] != "1.2.3" || got["commit"] != "abc123" || got["date"] != "2026-07-14T09:37:24Z" {
		t.Fatalf("version JSON metadata = %#v", got)
	}

	plain := testutil.RunCommand(t, "version", "--plain")
	if plain.ExitCode != cli.ExitSuccess || plain.Stderr != "" {
		t.Fatalf("version --plain result = %#v", plain)
	}
	if !strings.Contains(plain.Stdout, "version\t1.2.3\n") || !strings.Contains(plain.Stdout, "go_version\t") {
		t.Fatalf("version plain stdout = %q", plain.Stdout)
	}

	jq := testutil.RunCommand(t, "version", "--jq", ".version")
	if jq.ExitCode != cli.ExitSuccess || jq.Stderr != "" || jq.Stdout != "\"1.2.3\"\n" {
		t.Fatalf("version --jq result = %#v", jq)
	}

	tmpl := testutil.RunCommand(t, "version", "--template", "{{.version}}")
	if tmpl.ExitCode != cli.ExitSuccess || tmpl.Stderr != "" || tmpl.Stdout != "1.2.3\n" {
		t.Fatalf("version --template result = %#v", tmpl)
	}
}

type codedError struct {
	code int
}

func (e codedError) Error() string {
	return "coded"
}

func (e codedError) ExitCode() int {
	return e.code
}

func TestExitCodeFor(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", err: nil, want: cli.ExitSuccess},
		{name: "plain", err: errors.New("plain"), want: cli.ExitUsage},
		{name: "reserved", err: &cli.ReservedCommandError{Path: "chab reserved leaf"}, want: cli.ExitUsage},
		{name: "wrapped", err: fmt.Errorf("wrapped: %w", codedError{code: cli.ExitAuth}), want: cli.ExitAuth},
	}
	for _, test := range tests {
		if got := cli.ExitCodeFor(test.err); got != test.want {
			t.Fatalf("%s ExitCodeFor() = %d, want %d", test.name, got, test.want)
		}
	}
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
