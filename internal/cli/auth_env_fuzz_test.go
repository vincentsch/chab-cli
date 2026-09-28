package cli_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

func FuzzAuthenticationEnvironmentMatchesStrictRuntime(f *testing.F) {
	for _, seed := range [][]byte{
		{0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 3, 0, 0, 0, 2, 0, 1},
		{1, 2, 0, 3, 0, 2, 0, 2, 2},
		{2, 3, 0, 0, 3, 0, 0, 0, 3},
		{2, 3, 3, 0, 0, 0, 3, 0, 4},
		{1, 0, 5, 5, 5, 0, 0, 0, 1},
		{1, 0, 4, 4, 4, 0, 0, 0, 2},
		{3, 0, 0, 0, 0, 0, 0, 0, 1},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, selectors []byte) {
		if len(selectors) == 0 {
			selectors = []byte{0}
		}
		at := func(index, choices int) int {
			return int(selectors[index%len(selectors)]) % choices
		}

		state := testutil.NewState(t)
		configKind := at(0, 4)
		switch configKind {
		case 1:
			testutil.WriteRawConfig(t, state.ConfigPath, []byte(
				"version: 1\n"+
					"current_profile: local\n"+
					"profiles:\n"+
					"  local:\n"+
					"    base_url: https://file.example.test/base/\n"+
					"    api_base_url: https://file-api.example.test/v1/\n"+
					"    locale: en\n"+
					"    default_output: table\n"+
					"    defaults:\n"+
					"      project_list_limit: 37\n"+
					"  other:\n"+
					"    base_url: https://other.example.test/\n"+
					"    locale: de\n",
			))
		case 2:
			testutil.WriteRawConfig(t, state.ConfigPath, []byte(
				"version: 1\n"+
					"current_profile: absent\n"+
					"profiles:\n"+
					"  local:\n"+
					"    base_url: https://file.example.test\n",
			))
		case 3:
			testutil.WriteMalformedConfig(t, state.ConfigPath)
		}
		testutil.WriteMalformedAuth(t, state.AuthPath)
		configBefore, configExists := readOptionalFile(t, state.ConfigPath)
		authBefore, err := os.ReadFile(state.AuthPath)
		if err != nil {
			t.Fatal(err)
		}

		flags := config.FlagOverrides{
			ConfigPath: config.OverrideString{Value: state.ConfigPath, Set: true},
			AuthPath:   config.OverrideString{Value: state.AuthPath, Set: true},
		}
		args := state.Args()
		env := map[string]string{}

		switch at(1, 4) {
		case 1:
			env["CHAB_PROFILE"] = "other"
		case 2:
			flags.Profile = config.OverrideString{Value: "other", Set: true}
			args = append(args, "--profile", "other")
		case 3:
			env["CHAB_PROFILE"] = "ephemeral"
		}
		applyStringSource(
			at(2, 6),
			"CHAB_BASE_URL",
			"--base-url",
			[]string{"", "", "   ", "https://env-base.example.test/path/"},
			[]string{"", "", "https://flag-base.example.test/path/", "ftp://invalid.example.test"},
			env,
			&flags.BaseURL,
			&args,
		)
		applyStringSource(
			at(3, 6),
			"CHAB_API_BASE_URL",
			"--api-base-url",
			[]string{"", "", "   ", "https://env-api.example.test/v2/"},
			[]string{"", "", "https://flag-api.example.test/v3/", "relative"},
			env,
			&flags.APIBaseURL,
			&args,
		)
		applyStringSource(
			at(4, 6),
			"CHAB_LOCALE",
			"--locale",
			[]string{"", "", "en", "fr"},
			[]string{"", "", "de", "fr"},
			env,
			&flags.Locale,
			&args,
		)

		lookup := func(name string) (string, bool) {
			if name == "CHAB_API_KEY" {
				t.Fatalf("auth env looked up %s", name)
			}
			value, ok := env[name]
			return value, ok
		}
		wantRuntime, wantErr := config.ResolveRuntime(config.Options{
			Flags:     flags,
			LookupEnv: lookup,
			Mode:      config.ResolveStrict,
		})

		args = append(args, "auth", "env")
		mode := at(8, 5)
		switch mode {
		case 1:
			args = append(args, "--json")
		case 2:
			args = append(args, "--plain")
		case 3:
			args = append(args, "--jq", ".profile")
		case 4:
			args = append(args, "--template", "{{.profile}}")
		}
		input := &countingReader{}
		result := testutil.RunCommandWith(t, testutil.Options{
			Stdin:     input,
			LookupEnv: lookup,
		}, args...)
		if input.reads != 0 {
			t.Fatalf("stdin reads = %d, want 0", input.reads)
		}

		if wantErr != nil {
			var wantConfigErr *config.Error
			if !errors.As(wantErr, &wantConfigErr) {
				t.Fatalf("ResolveRuntime() error = %T %v", wantErr, wantErr)
			}
			assertAuthenticationEnvironmentConfigError(t, result, wantConfigErr.Kind)
		} else {
			if result.ExitCode != 0 || result.Err != nil {
				t.Fatalf("command result = %#v, want exit 0 without returned error", result)
			}
			assertFuzzedEnvironmentOutput(t, mode, result, wantRuntime)
		}

		configAfter, configStillExists := readOptionalFile(t, state.ConfigPath)
		if configExists != configStillExists || !bytes.Equal(configAfter, configBefore) {
			t.Fatalf("config changed: existed %t -> %t", configExists, configStillExists)
		}
		authAfter, err := os.ReadFile(state.AuthPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(authAfter, authBefore) {
			t.Fatalf("auth file changed")
		}
		assertDirectoryEntries(t, state.Dir, expectedStateEntries(configExists))
	})
}

func applyStringSource(
	selector int,
	envName string,
	flagName string,
	envValues []string,
	flagValues []string,
	env map[string]string,
	override *config.OverrideString,
	args *[]string,
) {
	switch selector {
	case 1:
		env[envName] = envValues[1]
	case 2:
		env[envName] = envValues[2]
	case 3:
		env[envName] = envValues[3]
	case 4:
		override.Value = flagValues[1]
		override.Set = true
		*args = append(*args, flagName, flagValues[1])
	case 5:
		override.Value = flagValues[2]
		override.Set = true
		*args = append(*args, flagName, flagValues[2])
	}
}

func assertFuzzedEnvironmentOutput(t *testing.T, mode int, result testutil.Result, runtime config.Runtime) {
	t.Helper()
	switch mode {
	case 0:
		want := "CI authentication environment\n" +
			"Profile: " + runtime.Profile + "\n" +
			"Base URL: " + runtime.BaseURL + "\n" +
			"API base URL: " + runtime.APIBaseURL + "\n"
		if runtime.Locale != "" {
			want += "Locale: " + runtime.Locale + "\n"
		}
		want += envGuidance
		if result.Stdout != want || result.Stderr != "" {
			t.Fatalf("human result = %#v, want stdout %q", result, want)
		}
	case 1:
		assertAuthenticationEnvironmentJSON(
			t,
			result.Stdout,
			runtime.Profile,
			runtime.BaseURL,
			runtime.APIBaseURL,
			runtime.Locale,
		)
		if result.Stderr != "" {
			t.Fatalf("JSON stderr = %q, want empty", result.Stderr)
		}
	case 2:
		want := "CHAB_PROFILE\t" + runtime.Profile + "\n" +
			"CHAB_BASE_URL\t" + runtime.BaseURL + "\n" +
			"CHAB_API_BASE_URL\t" + runtime.APIBaseURL + "\n"
		if runtime.Locale != "" {
			want += "CHAB_LOCALE\t" + runtime.Locale + "\n"
		}
		if result.Stdout != want || result.Stderr != envGuidance {
			t.Fatalf("plain result = %#v, want stdout %q", result, want)
		}
	case 3:
		if result.Stdout != strconv.Quote(runtime.Profile)+"\n" || result.Stderr != "" {
			t.Fatalf("jq result = %#v", result)
		}
	case 4:
		if result.Stdout != runtime.Profile+"\n" || result.Stderr != "" {
			t.Fatalf("template result = %#v", result)
		}
	default:
		t.Fatalf("unknown mode %d", mode)
	}
}

func readOptionalFile(t *testing.T, path string) ([]byte, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return data, true
}

func expectedStateEntries(configExists bool) []string {
	if configExists {
		return []string{"auth.json", "config.yml"}
	}
	return []string{"auth.json"}
}

func TestAuthenticationEnvironmentFuzzSeedsDescribeBoundedSources(t *testing.T) {
	env := map[string]string{}
	override := config.OverrideString{}
	args := []string{}
	applyStringSource(
		5,
		"CHAB_BASE_URL",
		"--base-url",
		[]string{"", "", "   ", "https://env.example.test"},
		[]string{"", "", "https://flag.example.test", "ftp://invalid.example.test"},
		env,
		&override,
		&args,
	)
	if len(env) != 0 ||
		!reflect.DeepEqual(override, config.OverrideString{Value: "https://flag.example.test", Set: true}) ||
		!reflect.DeepEqual(args, []string{"--base-url", "https://flag.example.test"}) {
		t.Fatalf("bounded source = env %#v override %#v args %#v", env, override, args)
	}
	if got := fmt.Sprint(expectedStateEntries(true)); got != "[auth.json config.yml]" {
		t.Fatalf("state entry order = %s", got)
	}
}
