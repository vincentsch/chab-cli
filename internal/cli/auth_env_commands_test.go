package cli_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/testutil"
)

const envGuidance = "Set CHAB_API_KEY from your CI provider's secret store.\n"

func TestAuthenticationEnvironmentReportsBuiltInRuntimeInEveryOutputMode(t *testing.T) {
	state := testutil.NewState(t)
	modes := []struct {
		name   string
		args   []string
		stdout string
		stderr string
	}{
		{
			name: "human",
			stdout: "CI authentication environment\n" +
				"Profile: local\n" +
				"Base URL: https://www.chab.ai\n" +
				"API base URL: https://www.chab.ai/v1\n" +
				envGuidance,
		},
		{
			name: "plain",
			args: []string{"--plain"},
			stdout: "CHAB_PROFILE\tlocal\n" +
				"CHAB_BASE_URL\thttps://www.chab.ai\n" +
				"CHAB_API_BASE_URL\thttps://www.chab.ai/v1\n",
			stderr: envGuidance,
		},
		{
			name: "JSON",
			args: []string{"--json"},
			stdout: "{\n" +
				"  \"profile\": \"local\",\n" +
				"  \"base_url\": \"https://www.chab.ai\",\n" +
				"  \"api_base_url\": \"https://www.chab.ai/v1\",\n" +
				"  \"locale\": \"\",\n" +
				"  \"secret_variable\": \"CHAB_API_KEY\"\n" +
				"}\n",
		},
		{name: "jq", args: []string{"--jq", ".api_base_url"}, stdout: "\"https://www.chab.ai/v1\"\n"},
		{name: "template", args: []string{"--template", "{{.profile}}|{{.locale}}"}, stdout: "local|\n"},
	}
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			args := append(state.Args(), "auth", "env")
			args = append(args, mode.args...)
			result := testutil.RunCommandWith(t, testutil.Options{LookupEnv: testutil.HermeticEnv(nil)}, args...)
			if result.ExitCode != cli.ExitSuccess || result.Err != nil ||
				result.Stdout != mode.stdout || result.Stderr != mode.stderr {
				t.Fatalf("result = %#v, want stdout=%q stderr=%q", result, mode.stdout, mode.stderr)
			}
			assertPathMissing(t, state.ConfigPath)
			assertPathMissing(t, state.AuthPath)
		})
	}
}

func TestAuthenticationEnvironmentUsesRuntimePrecedenceAndDerivation(t *testing.T) {
	state := testutil.NewState(t)
	testutil.WriteConfigProfile(t, state.ConfigPath, "saved", config.Profile{
		BaseURL:    "https://file.example.test/base/",
		APIBaseURL: "https://file-api.example.test/v1/",
		Locale:     "en",
	}, true)
	testutil.WriteConfigProfile(t, state.ConfigPath, "other", config.Profile{
		BaseURL:    "https://other.example.test/",
		APIBaseURL: "https://other-api.example.test/v1/",
		Locale:     "de",
	}, false)

	for _, test := range []struct {
		name    string
		env     map[string]string
		flags   []string
		profile string
		base    string
		api     string
		locale  string
	}{
		{
			name:    "selected saved profile",
			profile: "saved",
			base:    "https://file.example.test/base",
			api:     "https://file-api.example.test/v1",
			locale:  "en",
		},
		{
			name:    "explicit profile",
			flags:   []string{"--profile", "other"},
			profile: "other",
			base:    "https://other.example.test",
			api:     "https://other-api.example.test/v1",
			locale:  "de",
		},
		{
			name: "flags win over environment and file",
			env: map[string]string{
				"CHAB_PROFILE":      "other",
				"CHAB_BASE_URL":     "https://env.example.test/base",
				"CHAB_API_BASE_URL": "https://env-api.example.test/v1",
				"CHAB_LOCALE":       "de",
			},
			flags: []string{
				"--profile", "saved",
				"--base-url", "https://flag.example.test/base/",
				"--api-base-url", "https://flag-api.example.test/v1/",
				"--locale", "en",
			},
			profile: "saved",
			base:    "https://flag.example.test/base",
			api:     "https://flag-api.example.test/v1",
			locale:  "en",
		},
		{
			name: "environment profile wins over selected file profile",
			env: map[string]string{
				"CHAB_PROFILE": "other",
			},
			profile: "other",
			base:    "https://other.example.test",
			api:     "https://other-api.example.test/v1",
			locale:  "de",
		},
		{
			name: "environment wins over file",
			env: map[string]string{
				"CHAB_BASE_URL": "https://env.example.test/base/",
				"CHAB_LOCALE":   "de",
			},
			profile: "saved",
			base:    "https://env.example.test/base",
			api:     "https://env.example.test/base/v1",
			locale:  "de",
		},
		{
			name: "explicit API environment wins",
			env: map[string]string{
				"CHAB_BASE_URL":     "https://env.example.test/base/",
				"CHAB_API_BASE_URL": "https://explicit-api.example.test/v2/",
			},
			profile: "saved",
			base:    "https://env.example.test/base",
			api:     "https://explicit-api.example.test/v2",
			locale:  "en",
		},
		{
			name:    "higher precedence base URL re-derives API URL",
			flags:   []string{"--base-url", "https://flag.example.test/new/"},
			profile: "saved",
			base:    "https://flag.example.test/new",
			api:     "https://flag.example.test/new/v1",
			locale:  "en",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append(state.Args(), test.flags...)
			args = append(args, "auth", "env", "--json")
			result := testutil.RunCommandWith(t, testutil.Options{LookupEnv: testutil.HermeticEnv(test.env)}, args...)
			testutil.AssertSuccess(t, result)
			assertAuthenticationEnvironmentJSON(t, result.Stdout, test.profile, test.base, test.api, test.locale)
		})
	}
}

func TestAuthenticationEnvironmentSupportsUnsavedEnvironmentProfiles(t *testing.T) {
	state := testutil.NewState(t)
	testutil.WriteConfigProfile(t, state.ConfigPath, "saved", config.Profile{
		BaseURL: "https://saved.example.test",
	}, true)

	for _, test := range []struct {
		name string
		env  map[string]string
		base string
		api  string
	}{
		{
			name: "base URL override",
			env: map[string]string{
				"CHAB_PROFILE":  "ephemeral",
				"CHAB_BASE_URL": "https://ci.example.test/base/",
			},
			base: "https://ci.example.test/base",
			api:  "https://ci.example.test/base/v1",
		},
		{
			name: "API URL override",
			env: map[string]string{
				"CHAB_PROFILE":      "ephemeral",
				"CHAB_API_BASE_URL": "https://ci-api.example.test/v2/",
			},
			base: config.DefaultBaseURL,
			api:  "https://ci-api.example.test/v2",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := testutil.RunCommandWith(
				t,
				testutil.Options{LookupEnv: testutil.HermeticEnv(test.env)},
				state.Args("auth", "env", "--json")...,
			)
			testutil.AssertSuccess(t, result)
			assertAuthenticationEnvironmentJSON(t, result.Stdout, "ephemeral", test.base, test.api, "")
		})
	}
}

func TestAuthenticationEnvironmentPreservesStrictMissingProfileRules(t *testing.T) {
	for _, test := range []struct {
		name     string
		env      map[string]string
		flags    []string
		wantKind config.ErrorKind
		wantBase string
		wantAPI  string
	}{
		{name: "no override", wantKind: config.ErrMissingProfile},
		{
			name:     "base URL flag",
			flags:    []string{"--base-url", "https://flag.example.test/base"},
			wantBase: "https://flag.example.test/base",
			wantAPI:  "https://flag.example.test/base/v1",
		},
		{
			name:     "API URL flag",
			flags:    []string{"--api-base-url", "https://flag-api.example.test/v2"},
			wantBase: config.DefaultBaseURL,
			wantAPI:  "https://flag-api.example.test/v2",
		},
		{
			name: "base URL environment",
			env: map[string]string{
				"CHAB_BASE_URL": "https://env.example.test/base",
			},
			wantBase: "https://env.example.test/base",
			wantAPI:  "https://env.example.test/base/v1",
		},
		{
			name: "API URL environment",
			env: map[string]string{
				"CHAB_API_BASE_URL": "https://env-api.example.test/v2",
			},
			wantBase: config.DefaultBaseURL,
			wantAPI:  "https://env-api.example.test/v2",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := testutil.NewState(t)
			testutil.WriteRawConfig(t, state.ConfigPath, []byte(
				"version: 1\ncurrent_profile: absent\nprofiles:\n  saved:\n    base_url: https://saved.example.test\n",
			))
			args := append(state.Args(), test.flags...)
			args = append(args, "auth", "env", "--json")
			result := testutil.RunCommandWith(t, testutil.Options{LookupEnv: testutil.HermeticEnv(test.env)}, args...)
			if test.wantKind != "" {
				assertAuthenticationEnvironmentConfigError(t, result, test.wantKind)
				return
			}
			testutil.AssertSuccess(t, result)
			assertAuthenticationEnvironmentJSON(t, result.Stdout, "absent", test.wantBase, test.wantAPI, "")
		})
	}
}

func TestAuthenticationEnvironmentReturnsTypedRuntimeErrorsWithoutOutput(t *testing.T) {
	// Use an existing config with a missing selection so invalid explicit
	// overrides must pass the missing-profile gate before validation fails.
	writeMissingSelection := func(t *testing.T, state testutil.State) {
		testutil.WriteRawConfig(t, state.ConfigPath, []byte(
			"version: 1\ncurrent_profile: absent\nprofiles:\n  saved:\n    base_url: https://saved.example.test\n",
		))
	}
	for _, test := range []struct {
		name  string
		setup func(*testing.T, testutil.State)
		env   map[string]string
		flags []string
		kind  config.ErrorKind
	}{
		{
			name:  "malformed config",
			setup: func(t *testing.T, state testutil.State) { testutil.WriteMalformedConfig(t, state.ConfigPath) },
			kind:  config.ErrMalformedConfig,
		},
		{name: "invalid profile", flags: []string{"--profile", "INVALID"}, kind: config.ErrInvalidProfileName},
		{name: "invalid base URL", flags: []string{"--base-url", "ftp://example.test"}, kind: config.ErrInvalidURL},
		{name: "invalid API URL", flags: []string{"--api-base-url", "relative"}, kind: config.ErrInvalidURL},
		{name: "invalid locale", flags: []string{"--locale", "fr"}, kind: config.ErrInvalidLocale},
		{
			name:  "empty base URL flag bypasses missing profile gate",
			setup: writeMissingSelection,
			flags: []string{"--base-url="},
			kind:  config.ErrInvalidURL,
		},
		{
			name:  "empty API URL flag bypasses missing profile gate",
			setup: writeMissingSelection,
			flags: []string{"--api-base-url="},
			kind:  config.ErrInvalidURL,
		},
		{
			name:  "whitespace base URL environment bypasses missing profile gate",
			setup: writeMissingSelection,
			env:   map[string]string{"CHAB_BASE_URL": "   "},
			kind:  config.ErrInvalidURL,
		},
		{
			name:  "whitespace API URL environment bypasses missing profile gate",
			setup: writeMissingSelection,
			env:   map[string]string{"CHAB_API_BASE_URL": "   "},
			kind:  config.ErrInvalidURL,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := testutil.NewState(t)
			if test.setup != nil {
				test.setup(t, state)
			}
			args := append(state.Args(), test.flags...)
			args = append(args, "auth", "env", "--json")
			result := testutil.RunCommandWith(t, testutil.Options{LookupEnv: testutil.HermeticEnv(test.env)}, args...)
			assertAuthenticationEnvironmentConfigError(t, result, test.kind)
		})
	}
}

func TestAuthenticationEnvironmentTreatsEmptyEnvironmentOverridesAsUnset(t *testing.T) {
	for _, name := range []string{"CHAB_BASE_URL", "CHAB_API_BASE_URL"} {
		t.Run(name, func(t *testing.T) {
			state := testutil.NewState(t)
			testutil.WriteRawConfig(t, state.ConfigPath, []byte(
				"version: 1\ncurrent_profile: absent\nprofiles:\n  saved:\n    base_url: https://saved.example.test\n",
			))
			result := testutil.RunCommandWith(
				t,
				testutil.Options{LookupEnv: testutil.HermeticEnv(map[string]string{name: ""})},
				state.Args("auth", "env", "--json")...,
			)
			assertAuthenticationEnvironmentConfigError(t, result, config.ErrMissingProfile)
		})
	}

	t.Run("empty locale environment is unset", func(t *testing.T) {
		state := testutil.NewState(t)
		result := testutil.RunCommandWith(
			t,
			testutil.Options{LookupEnv: testutil.HermeticEnv(map[string]string{"CHAB_LOCALE": ""})},
			state.Args("auth", "env", "--json")...,
		)
		testutil.AssertSuccess(t, result)
		assertAuthenticationEnvironmentJSON(t, result.Stdout, "local", config.DefaultBaseURL, config.DefaultAPIBaseURL, "")
	})

	t.Run("empty locale flag remains explicit", func(t *testing.T) {
		state := testutil.NewState(t)
		result := testutil.RunCommandWith(
			t,
			testutil.Options{LookupEnv: testutil.HermeticEnv(nil)},
			state.Args("--locale=", "auth", "env", "--json")...,
		)
		assertAuthenticationEnvironmentConfigError(t, result, config.ErrInvalidLocale)
	})
}

func TestAuthenticationEnvironmentRejectsInvalidURLStructures(t *testing.T) {
	for _, flag := range []string{"--base-url", "--api-base-url"} {
		for _, value := range []string{
			"https://user@example.test",
			"https://ci.example.test?mode=qa",
			"https://ci.example.test/#fragment",
			"https:///path",
			"relative/path",
			"ftp://ci.example.test",
		} {
			t.Run(flag+"/"+value, func(t *testing.T) {
				state := testutil.NewState(t)
				result := testutil.RunCommandWith(
					t,
					testutil.Options{LookupEnv: testutil.HermeticEnv(nil)},
					state.Args(flag, value, "auth", "env", "--json")...,
				)
				assertAuthenticationEnvironmentConfigError(t, result, config.ErrInvalidURL)
				assertPathMissing(t, state.ConfigPath)
				assertPathMissing(t, state.AuthPath)
			})
		}
	}
}

func TestAuthenticationEnvironmentValidatesCompleteStrictRuntime(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{
			name: "output default",
			body: "version: 1\ncurrent_profile: saved\nprofiles:\n  saved:\n" +
				"    base_url: https://saved.example.test\n    default_output: json\n",
		},
		{
			name: "project list default",
			body: "version: 1\ncurrent_profile: saved\nprofiles:\n  saved:\n" +
				"    base_url: https://saved.example.test\n    defaults:\n      project_list_limit: 0\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := testutil.NewState(t)
			testutil.WriteRawConfig(t, state.ConfigPath, []byte(test.body))
			result := testutil.RunCommandWith(
				t,
				testutil.Options{LookupEnv: testutil.HermeticEnv(nil)},
				state.Args("auth", "env", "--json")...,
			)
			assertAuthenticationEnvironmentConfigError(t, result, config.ErrInvalidDefault)
		})
	}

	t.Run("valid hidden defaults remain absent from report", func(t *testing.T) {
		state := testutil.NewState(t)
		testutil.WriteRawConfig(t, state.ConfigPath, []byte(
			"version: 1\ncurrent_profile: saved\nprofiles:\n  saved:\n"+
				"    base_url: https://saved.example.test\n"+
				"    default_output: table\n"+
				"    defaults:\n      project_list_limit: 37\n",
		))
		result := testutil.RunCommandWith(
			t,
			testutil.Options{LookupEnv: testutil.HermeticEnv(nil)},
			state.Args("auth", "env", "--json")...,
		)
		testutil.AssertSuccess(t, result)
		assertAuthenticationEnvironmentJSON(
			t,
			result.Stdout,
			"saved",
			"https://saved.example.test",
			"https://saved.example.test/v1",
			"",
		)
	})
}

func TestAuthenticationEnvironmentValidatesPathsWithoutReadingAuthContent(t *testing.T) {
	t.Run("empty config path", func(t *testing.T) {
		state := testutil.NewState(t)
		result := testutil.RunCommandWith(
			t,
			testutil.Options{LookupEnv: testutil.HermeticEnv(nil)},
			"--config=", "--auth-file", state.AuthPath, "auth", "env", "--json",
		)
		assertAuthenticationEnvironmentConfigError(t, result, config.ErrMalformedConfig)
	})

	t.Run("empty auth path", func(t *testing.T) {
		state := testutil.NewState(t)
		result := testutil.RunCommandWith(
			t,
			testutil.Options{LookupEnv: testutil.HermeticEnv(nil)},
			"--config", state.ConfigPath, "--auth-file=", "auth", "env", "--json",
		)
		assertAuthenticationEnvironmentConfigError(t, result, config.ErrMalformedConfig)
	})

	t.Run("config path can also be the unread auth object", func(t *testing.T) {
		state := testutil.NewState(t)
		testutil.WriteRawConfig(t, state.ConfigPath, []byte(
			"version: 1\ncurrent_profile: saved\nprofiles:\n  saved:\n    base_url: https://saved.example.test\n",
		))
		before, err := os.ReadFile(state.ConfigPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, authPath := range []string{
			state.ConfigPath,
			filepath.Join(state.Dir, "config-auth-symlink"),
			filepath.Join(state.Dir, "config-auth-hardlink"),
		} {
			switch filepath.Base(authPath) {
			case "config-auth-symlink":
				if err := os.Symlink(state.ConfigPath, authPath); err != nil {
					t.Fatal(err)
				}
			case "config-auth-hardlink":
				if err := os.Link(state.ConfigPath, authPath); err != nil {
					t.Fatal(err)
				}
			}
			result := testutil.RunCommandWith(
				t,
				testutil.Options{LookupEnv: testutil.HermeticEnv(nil)},
				"--config", state.ConfigPath,
				"--auth-file", authPath,
				"auth", "env", "--json",
			)
			testutil.AssertSuccess(t, result)
			assertAuthenticationEnvironmentJSON(
				t,
				result.Stdout,
				"saved",
				"https://saved.example.test",
				"https://saved.example.test/v1",
				"",
			)
		}
		after, err := os.ReadFile(state.ConfigPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(after, before) {
			t.Fatalf("shared config/auth object changed")
		}
	})
}

func TestAuthenticationEnvironmentPreflightAndFalseFlagsAvoidRuntimeWork(t *testing.T) {
	state := testutil.NewState(t)
	testutil.WriteMalformedConfig(t, state.ConfigPath)
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "plain and JSON", args: []string{"--plain", "--json"}, want: "--plain cannot be used"},
		{name: "plain and jq", args: []string{"--plain", "--jq", "."}, want: "--plain cannot be used"},
		{name: "plain and template", args: []string{"--plain", "--template", "{{.profile}}"}, want: "--plain cannot be used"},
		{name: "jq and template", args: []string{"--jq", ".", "--template", "{{.profile}}"}, want: "--jq and --template cannot be used together"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := state.Args("auth", "env")
			args = append(args, test.args...)
			result := testutil.RunCommandWith(
				t,
				testutil.Options{LookupEnv: testutil.HermeticEnv(nil)},
				args...,
			)
			if result.ExitCode != cli.ExitUsage || result.Stdout != "" || result.Err == nil ||
				!strings.Contains(result.Err.Error(), test.want) {
				t.Fatalf("result = %#v, want preflight error containing %q", result, test.want)
			}
			var configErr *config.Error
			if errors.As(result.Err, &configErr) {
				t.Fatalf("mode preflight reached config: %v", result.Err)
			}
		})
	}

	for _, test := range []struct {
		name string
		args []string
		json bool
	}{
		{name: "plain false", args: []string{"--plain=false"}},
		{name: "JSON false", args: []string{"--json=false"}},
		{name: "metadata false", args: []string{"--json", "--include-meta=false"}, json: true},
		{name: "debug false", args: []string{"--json", "--debug=false"}, json: true},
		{name: "empty jq identity", args: []string{"--jq="}, json: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			missingState := testutil.NewState(t)
			args := missingState.Args("auth", "env")
			args = append(args, test.args...)
			result := testutil.RunCommandWith(
				t,
				testutil.Options{LookupEnv: testutil.HermeticEnv(nil)},
				args...,
			)
			testutil.AssertSuccess(t, result)
			if test.json {
				assertAuthenticationEnvironmentJSON(t, result.Stdout, "local", config.DefaultBaseURL, config.DefaultAPIBaseURL, "")
			} else if !strings.HasPrefix(result.Stdout, "CI authentication environment\n") {
				t.Fatalf("false mode flag selected another renderer: %#v", result)
			}
			assertPathMissing(t, missingState.ConfigPath)
			assertPathMissing(t, missingState.AuthPath)
		})
	}
}

func TestAuthenticationEnvironmentRejectsUnsupportedInvocationBeforeMalformedRuntime(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "response metadata",
			args: []string{"auth", "env", "--json", "--include-meta"},
			want: `--include-meta is not supported for "chab auth env"`,
		},
		{
			name: "extra argument",
			args: []string{"auth", "env", "extra"},
			want: `unknown command "extra" for "chab auth env"`,
		},
		{
			name: "dry run",
			args: []string{"auth", "env", "--dry-run"},
			want: "unknown flag: --dry-run",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := testutil.NewState(t)
			testutil.WriteMalformedConfig(t, state.ConfigPath)
			configBefore, err := os.ReadFile(state.ConfigPath)
			if err != nil {
				t.Fatal(err)
			}

			input := &countingReader{}
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "unexpected", http.StatusInternalServerError)
			})
			credentialLookups := 0
			lookup := func(name string) (string, bool) {
				if name == "CHAB_API_KEY" {
					credentialLookups++
				}
				return "", false
			}

			args := state.Args("--base-url", server.URL)
			args = append(args, test.args...)
			result := testutil.RunCommandWith(t, testutil.Options{
				Stdin:     input,
				LookupEnv: lookup,
			}, args...)
			if result.ExitCode != cli.ExitUsage || result.Stdout != "" || result.Err == nil ||
				!strings.Contains(result.Err.Error(), test.want) {
				t.Fatalf("result = %#v, want local rejection containing %q", result, test.want)
			}
			var configErr *config.Error
			if errors.As(result.Err, &configErr) {
				t.Fatalf("unsupported invocation reached malformed config: %v", result.Err)
			}
			if credentialLookups != 0 {
				t.Fatalf("credential lookups = %d, want 0", credentialLookups)
			}
			if input.reads != 0 {
				t.Fatalf("stdin reads = %d, want 0", input.reads)
			}
			configAfter, err := os.ReadFile(state.ConfigPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(configAfter, configBefore) {
				t.Fatalf("malformed config changed")
			}
			assertPathMissing(t, state.AuthPath)
			assertDirectoryEntries(t, state.Dir, []string{filepath.Base(state.ConfigPath)})
			server.AssertNoRequests(t)
		})
	}
}

func TestAuthenticationEnvironmentTransformFailuresAreAtomicAndLocal(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "jq runtime", args: []string{"--jq", `error("transform failure")`}, want: "transform failure"},
		{name: "template parse", args: []string{"--template", "{{"}, want: "template"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := testutil.NewState(t)
			testutil.WriteMalformedAuth(t, state.AuthPath)
			authBefore, err := os.ReadFile(state.AuthPath)
			if err != nil {
				t.Fatal(err)
			}
			input := &countingReader{}
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "unexpected", http.StatusInternalServerError)
			})
			args := state.Args("--base-url", server.URL, "auth", "env")
			args = append(args, test.args...)
			result := testutil.RunCommandWith(t, testutil.Options{
				Stdin:     input,
				LookupEnv: testutil.HermeticEnv(nil),
			}, args...)
			if result.ExitCode != cli.ExitUsage || result.Stdout != "" || result.Err == nil ||
				!strings.Contains(result.Err.Error(), test.want) {
				t.Fatalf("result = %#v, want atomic local transform failure containing %q", result, test.want)
			}
			if input.reads != 0 {
				t.Fatalf("stdin reads = %d, want 0", input.reads)
			}
			authAfter, err := os.ReadFile(state.AuthPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(authAfter, authBefore) {
				t.Fatalf("auth file changed")
			}
			assertPathMissing(t, state.ConfigPath)
			server.AssertNoRequests(t)
		})
	}
}

func TestAuthenticationEnvironmentKeepsTerminalControlsInert(t *testing.T) {
	for _, value := range []string{
		"https://ci.example.test/\rforged",
		"https://ci.example.test/\nforged",
		"https://ci.example.test/\tforged",
		"https://ci.example.test/\x1b[31m",
		"https://ci.example.test/\x7fforged",
	} {
		t.Run("reject/"+strings.TrimPrefix(value, "https://ci.example.test/"), func(t *testing.T) {
			state := testutil.NewState(t)
			result := testutil.RunCommandWith(
				t,
				testutil.Options{LookupEnv: testutil.HermeticEnv(nil)},
				state.Args("--base-url", value, "auth", "env", "--json")...,
			)
			assertAuthenticationEnvironmentConfigError(t, result, config.ErrInvalidURL)
			diagnostics := []string{
				strings.TrimSuffix(result.Stderr, "\n"),
				errorText(result.Err),
			}
			for _, diagnostic := range diagnostics {
				for _, control := range []byte{'\r', '\n', '\t', 0x1b, 0x7f} {
					if strings.ContainsRune(diagnostic, rune(control)) {
						t.Fatalf("diagnostic contains raw control 0x%02x: %q", control, diagnostic)
					}
				}
			}
		})
	}

	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "percent encoded", input: "https://ci.example.test/%1B%5B31m", want: "https://ci.example.test/%1B%5B31m"},
		{name: "C1", input: "https://ci.example.test/\u0085tail", want: "https://ci.example.test/%C2%85tail"},
	} {
		for _, mode := range []struct {
			name string
			args []string
		}{
			{name: "human"},
			{name: "plain", args: []string{"--plain"}},
			{name: "JSON", args: []string{"--json"}},
			{name: "jq", args: []string{"--jq", ".base_url"}},
			{name: "template", args: []string{"--template", "{{.base_url}}"}},
		} {
			t.Run(test.name+"/"+mode.name, func(t *testing.T) {
				state := testutil.NewState(t)
				args := state.Args("--base-url", test.input, "auth", "env")
				args = append(args, mode.args...)
				result := testutil.RunCommandWith(
					t,
					testutil.Options{LookupEnv: testutil.HermeticEnv(nil)},
					args...,
				)
				if result.ExitCode != cli.ExitSuccess || result.Err != nil ||
					!strings.Contains(result.Stdout, test.want) {
					t.Fatalf("result = %#v, want inert URL %q", result, test.want)
				}
				if strings.Contains(result.Stdout+result.Stderr, "\x1b") ||
					strings.Contains(result.Stdout+result.Stderr, "\u0085") {
					t.Fatalf("result contains terminal-active control: %#v", result)
				}
			})
		}
	}
}

func TestAuthenticationEnvironmentStopsBeforeCredentialStateInputAndNetwork(t *testing.T) {
	// These candidate credentials are deliberately never returned. The lookup
	// closure is the boundary check: requesting the credential fails at once.
	// A separate test injects key-shaped runtime text to cover actual leakage.
	for _, protected := range []string{"opaque-would-be-credential", testutil.FakeKey("env_forbidden_lookup")} {
		t.Run(protected[:3], func(t *testing.T) {
			state := testutil.NewState(t)
			// Each sentinel exposes a different forbidden side effect: auth
			// parsing, stdin consumption, network access, or local file writes.
			testutil.WriteMalformedAuth(t, state.AuthPath)
			before, err := os.ReadFile(state.AuthPath)
			if err != nil {
				t.Fatal(err)
			}
			beforeInfo, err := os.Stat(state.AuthPath)
			if err != nil {
				t.Fatal(err)
			}
			input := &countingReader{}
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "unexpected", http.StatusInternalServerError)
			})
			lookup := func(name string) (string, bool) {
				if name == "CHAB_API_KEY" {
					t.Fatalf("auth env looked up %s", name)
				}
				return "", false
			}
			result := testutil.RunCommandWith(t, testutil.Options{
				Stdin:     input,
				LookupEnv: lookup,
			}, state.Args("--base-url", server.URL, "auth", "env", "--json")...)
			testutil.AssertSuccess(t, result)
			if input.reads != 0 {
				t.Fatalf("stdin reads = %d, want 0", input.reads)
			}
			after, err := os.ReadFile(state.AuthPath)
			if err != nil {
				t.Fatal(err)
			}
			afterInfo, err := os.Stat(state.AuthPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) || afterInfo.Mode() != beforeInfo.Mode() {
				t.Fatalf("auth file changed")
			}
			assertPathMissing(t, state.ConfigPath)
			assertDirectoryEntries(t, state.Dir, []string{filepath.Base(state.AuthPath)})
			server.AssertNoRequests(t)
			for _, value := range []string{protected, testutil.FakeKey("env_unreturned_peer")} {
				haystack := result.Stdout + result.Stderr + errorText(result.Err)
				testutil.AssertNoSecretValue(t, haystack, value)
				if strings.Contains(value, "|") {
					testutil.AssertNoSecret(t, haystack, value)
				}
			}
		})
	}
}

func TestAuthenticationEnvironmentRedactsKeyShapedRuntimeAcrossEveryOutputMode(t *testing.T) {
	key := testutil.FakeKey("env_runtime_path")
	for _, mode := range []struct {
		name string
		args []string
	}{
		{name: "human"},
		{name: "plain", args: []string{"--plain"}},
		{name: "JSON debug", args: []string{"--json", "--debug"}},
		{name: "jq", args: []string{"--jq", ".base_url"}},
		{name: "template", args: []string{"--template", "{{.base_url}}"}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			state := testutil.NewState(t)
			testutil.WriteMalformedAuth(t, state.AuthPath)
			authBefore, err := os.ReadFile(state.AuthPath)
			if err != nil {
				t.Fatal(err)
			}
			input := &countingReader{}
			server := testutil.NewAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "unexpected", http.StatusInternalServerError)
			})
			baseURL := server.URL + "/" + key
			lookup := func(name string) (string, bool) {
				if name == "CHAB_API_KEY" {
					t.Fatalf("auth env looked up %s", name)
				}
				return "", false
			}
			args := state.Args("--base-url", baseURL, "auth", "env")
			args = append(args, mode.args...)
			result := testutil.RunCommandWith(t, testutil.Options{
				Stdin:     input,
				LookupEnv: lookup,
			}, args...)
			if result.ExitCode != cli.ExitSuccess || result.Err != nil {
				t.Fatalf("result = %#v", result)
			}
			if !strings.Contains(result.Stdout+result.Stderr, "[REDACTED]") {
				t.Fatalf("result lacks redaction marker: %#v", result)
			}
			haystack := result.Stdout + result.Stderr + errorText(result.Err)
			testutil.AssertNoSecret(t, haystack, key)
			testutil.AssertNoSecretValue(t, haystack, key)
			if input.reads != 0 {
				t.Fatalf("stdin reads = %d, want 0", input.reads)
			}
			authAfter, err := os.ReadFile(state.AuthPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(authAfter, authBefore) {
				t.Fatalf("auth file changed")
			}
			assertPathMissing(t, state.ConfigPath)
			assertDirectoryEntries(t, state.Dir, []string{filepath.Base(state.AuthPath)})
			server.AssertNoRequests(t)

			if mode.name == "JSON debug" {
				// The key-shape matcher treats the remaining URL path, including
				// the derived /v1 suffix, as sensitive. Both URL fields
				// therefore collapse to the same sanitized value.
				safeBase := server.URL + "/ak_env_runtime_path%7C[REDACTED]"
				want := "{\n" +
					"  \"profile\": \"local\",\n" +
					"  \"base_url\": \"" + safeBase + "\",\n" +
					"  \"api_base_url\": \"" + safeBase + "\",\n" +
					"  \"locale\": \"\",\n" +
					"  \"secret_variable\": \"CHAB_API_KEY\"\n" +
					"}\n"
				if result.Stdout != want || result.Stderr != "" {
					t.Fatalf("debug JSON result = %#v, want stdout %q and empty stderr", result, want)
				}
			}
		})
	}
}

func assertAuthenticationEnvironmentJSON(t *testing.T, raw, profile, baseURL, apiBaseURL, locale string) {
	t.Helper()
	object := testutil.ParseJSONObject(t, raw)
	testutil.AssertKeys(t, object, []string{"profile", "base_url", "api_base_url", "locale", "secret_variable"})
	want := map[string]any{
		"profile":         profile,
		"base_url":        baseURL,
		"api_base_url":    apiBaseURL,
		"locale":          locale,
		"secret_variable": "CHAB_API_KEY",
	}
	if !reflect.DeepEqual(object, want) {
		t.Fatalf("report = %#v, want %#v", object, want)
	}
}

func assertAuthenticationEnvironmentConfigError(t *testing.T, result testutil.Result, kind config.ErrorKind) {
	t.Helper()
	if result.ExitCode != cli.ExitUsage || result.Stdout != "" || result.Err == nil {
		t.Fatalf("result = %#v, want local config failure with empty stdout", result)
	}
	var configErr *config.Error
	if !errors.As(result.Err, &configErr) || configErr.Kind != kind {
		t.Fatalf("error = %v, kind = %v, want %s", result.Err, configErr, kind)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s exists or stat failed: %v", path, err)
	}
}

func assertDirectoryEntries(t *testing.T, dir string, want []string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("directory entries = %v, want %v", got, want)
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// countingReader makes any unexpected stdin access observable to a test.
type countingReader struct {
	reads int
}

func (r *countingReader) Read([]byte) (int, error) {
	r.reads++
	return 0, io.ErrUnexpectedEOF
}
