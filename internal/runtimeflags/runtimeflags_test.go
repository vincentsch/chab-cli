package runtimeflags_test

import (
	"io"
	"reflect"
	"testing"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/runtimeflags"
)

func TestOverridesFromParsedRootFlags(t *testing.T) {
	got := executeAndReadOverrides(t,
		"--profile", "ci",
		"--config", "/tmp/chab config.yml",
		"--auth-file", "/tmp/chab auth.json",
		"--base-url", "https://example.test/app",
		"--api-base-url", "https://api.example.test/v1",
		"--locale", "de",
		"version",
	)
	want := config.FlagOverrides{
		Profile:    config.OverrideString{Value: "ci", Set: true},
		ConfigPath: config.OverrideString{Value: "/tmp/chab config.yml", Set: true},
		AuthPath:   config.OverrideString{Value: "/tmp/chab auth.json", Set: true},
		BaseURL:    config.OverrideString{Value: "https://example.test/app", Set: true},
		APIBaseURL: config.OverrideString{Value: "https://api.example.test/v1", Set: true},
		Locale:     config.OverrideString{Value: "de", Set: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Overrides() = %#v, want %#v", got, want)
	}
}

func TestOverridesPreserveExplicitEmptyFlags(t *testing.T) {
	got := executeAndReadOverrides(t,
		"--profile=",
		"--config=",
		"--auth-file=",
		"--base-url=",
		"--api-base-url=",
		"--locale=",
		"version",
	)
	want := config.FlagOverrides{
		Profile:    config.OverrideString{Value: "", Set: true},
		ConfigPath: config.OverrideString{Value: "", Set: true},
		AuthPath:   config.OverrideString{Value: "", Set: true},
		BaseURL:    config.OverrideString{Value: "", Set: true},
		APIBaseURL: config.OverrideString{Value: "", Set: true},
		Locale:     config.OverrideString{Value: "", Set: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Overrides() = %#v, want %#v", got, want)
	}
}

func TestOverridesUnsetFlagsStayUnset(t *testing.T) {
	got := executeAndReadOverrides(t, "version")
	if !reflect.DeepEqual(got, config.FlagOverrides{}) {
		t.Fatalf("Overrides() = %#v, want zero overrides", got)
	}
}

func executeAndReadOverrides(t *testing.T, args ...string) config.FlagOverrides {
	t.Helper()
	root := cli.NewRootCommand(io.Discard, io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(%v) error = %v", args, err)
	}
	// Read after executing the real root command so the test covers Cobra's
	// parsed values and Changed bits, including explicit empty strings.
	return runtimeflags.Overrides(root)
}
