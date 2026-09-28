package examplecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	testBinaryDir string
	testViltBin   string
	testMockBin   string
	testShimBin   string
)

// TestMain builds the three real executables once so boundary tests exercise
// production processes without paying the build cost in every subtest.
func TestMain(m *testing.M) {
	repoRoot, err := repoRootFromWorkingDirectory()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	testBinaryDir, err = os.MkdirTemp("", "examplecheck-tests-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	testViltBin = filepath.Join(testBinaryDir, exeName("chab"))
	testMockBin = filepath.Join(testBinaryDir, exeName("mockapi"))
	testShimBin = filepath.Join(testBinaryDir, exeName("chab-example-shim"))
	for _, build := range []struct {
		out string
		pkg string
	}{
		{out: testViltBin, pkg: "./cmd/chab"},
		{out: testMockBin, pkg: "./examples/mockapi"},
		{out: testShimBin, pkg: "./scripts/example-invocation-shim"},
	} {
		if err := buildBinary(context.Background(), repoRoot, filepath.Join(testBinaryDir, "go-build"), build.out, build.pkg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			_ = os.RemoveAll(testBinaryDir)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(testBinaryDir)
	os.Exit(code)
}

func TestRunExampleRealBoundarySuccessAndExitPropagation(t *testing.T) {
	skipWindowsShellExecution(t)
	tests := []struct {
		name    string
		body    string
		example Example
	}{
		{
			name: "success",
			body: `"$CHAB_BIN" api get /credits --json`,
			example: testExample(0, 1, []string{"spendable_balance"}, nil, []CommandUse{{
				Path: []string{"api", "get"}, Flags: []string{"json"},
			}}),
		},
		{
			name: "not found exit",
			body: `"$CHAB_BIN" api get /missing --json`,
			example: testExample(5, 1, nil, []string{`"code":"not_found"`}, []CommandUse{{
				Path: []string{"api", "get"}, Flags: []string{"json"},
			}}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := temporaryScriptRoot(t, "examples/ci/run.sh", tt.body)
			r := realTestRunner()
			errs := r.runExample(root, t.TempDir(), 0, tt.example)
			if len(errs) != 0 {
				t.Fatalf("runExample errors:\n%s", joinErrors(errs))
			}
		})
	}
}

func TestRunnerTemporaryRepositoryUsesBuiltInvocationShim(t *testing.T) {
	skipWindowsShellExecution(t)
	root := temporaryScriptRoot(t, "examples/ci/read-credits.sh", `
credits=$("${CHAB_BIN:-chab}" api get /credits --json)
printf '%s\n' "$credits"
`)
	writeTestFile(t, root, "go.mod", []byte("module example.test/automation\n\ngo 1.22\n"), 0o644)
	manifest := `{
  "examples": [{
    "script": "examples/ci/read-credits.sh",
    "description": "Read credits through the built invocation boundary.",
    "exit": 0,
    "stdout_contains": ["spendable_balance"],
    "mock_requests": 1,
    "commands": [{
      "path": ["api", "get"],
      "flags": ["json"]
    }]
  }]
}`
	writeTestFile(t, root, manifestPath, []byte(manifest), 0o644)

	var report bytes.Buffer
	r := realTestRunner()
	r.opts = RunOptions{RepoRoot: root, Stderr: &report}
	if err := r.run(); err != nil {
		t.Fatalf("runner error = %v\n%s", err, report.String())
	}
	if got := report.String(); got != "ok examples/ci/read-credits.sh\n" {
		t.Fatalf("runner report = %q", got)
	}
}

func TestRunnerUsesFreshSecretsMockAndStateForEveryExample(t *testing.T) {
	skipWindowsShellExecution(t)
	for _, failMiddle := range []bool{false, true} {
		name := "success"
		if failMiddle {
			name = "middle failure"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, root, "go.mod", []byte("module example.test/automation\n\ngo 1.22\n"), 0o644)

			specs := []struct {
				path  string
				index int
			}{
				{path: "examples/agents/write.sh", index: 1},
				{path: "examples/ci/read.sh", index: 2},
				{path: "examples/support/inspect.sh", index: 3},
			}
			manifest := Manifest{Examples: make([]Example, 0, len(specs))}
			for _, spec := range specs {
				finalStatus := ""
				if failMiddle && spec.index == 2 {
					finalStatus = "exit 17"
				}
				body := fmt.Sprintf(`#!/bin/sh
set -eu
test "$CHAB_CONFIG" = "$HOME/config.yml"
test "$CHAB_AUTH_FILE" = "$HOME/auth.json"
test "$CHAB_EXAMPLE_TRACE_PATH" = "$HOME/invocations.ndjson"
case "$HOME" in
  */state/%03d) ;;
  *) exit 91 ;;
esac
test ! -e "$HOME/state-marker"
printf 'owned\n' > "$HOME/state-marker"
"${CHAB_BIN:-chab}" api post /projects --field name=Fresh --dry-run --json --no-prompt
%s
`, spec.index, finalStatus)
				writeTestFile(t, root, spec.path, []byte(body), 0o755)
				manifest.Examples = append(manifest.Examples, Example{
					Script:         spec.path,
					Description:    "Exercise isolated state through the invocation boundary.",
					Exit:           0,
					StdoutContains: []string{`"method": "POST"`},
					MockRequests:   intPtr(0),
					Commands: []CommandUse{{
						Path:  []string{"api", "post"},
						Flags: []string{"dry-run", "field", "json", "no-prompt"},
					}},
				})
			}
			encodedManifest, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, root, manifestPath, encodedManifest, 0o644)

			issued := []exampleSecrets{
				{apiKey: "api-value-one", idempotencyKey: `idem-"-one`},
				{apiKey: "api-value-two", idempotencyKey: `idem-"-two`},
				{apiKey: "api-value-three", idempotencyKey: `idem-"-three`},
			}
			var mu sync.Mutex
			generatorCalls := 0
			startCalls := 0
			requestCalls := 0
			stopCalls := 0
			var observed []mockExpectations

			var report bytes.Buffer
			r := &runner{
				opts:           RunOptions{RepoRoot: root, Stderr: &report},
				viltBin:        testViltBin,
				mockBin:        testMockBin,
				shimBin:        testShimBin,
				exampleTimeout: 5 * time.Second,
			}
			r.generateSecrets = func() (exampleSecrets, error) {
				mu.Lock()
				defer mu.Unlock()
				if generatorCalls >= len(issued) {
					return exampleSecrets{}, fmt.Errorf("secret generator called too often")
				}
				value := issued[generatorCalls]
				generatorCalls++
				return value, nil
			}
			r.startMock = func(_ io.Writer, expectations mockExpectations) (mockHandle, error) {
				mu.Lock()
				startCalls++
				observed = append(observed, expectations)
				mu.Unlock()
				return mockHandle{
					baseURL: "http://example.test",
					requests: func() (int, error) {
						mu.Lock()
						requestCalls++
						mu.Unlock()
						return 0, nil
					},
					stop: func() error {
						mu.Lock()
						stopCalls++
						mu.Unlock()
						return nil
					},
				}, nil
			}

			runErr := r.run()
			if failMiddle {
				if runErr == nil || !strings.Contains(report.String(), "fail examples/ci/read.sh") {
					t.Fatalf("middle failure was not reported: err=%v report=%q", runErr, report.String())
				}
				for _, path := range []string{specs[0].path, specs[2].path} {
					if !strings.Contains(report.String(), "ok "+path+"\n") {
						t.Fatalf("later lifecycle did not complete for %s:\n%s", path, report.String())
					}
				}
			} else {
				if runErr != nil {
					t.Fatalf("runner error = %v\n%s", runErr, report.String())
				}
				want := "ok examples/agents/write.sh\nok examples/ci/read.sh\nok examples/support/inspect.sh\n"
				if report.String() != want {
					t.Fatalf("runner report = %q, want %q", report.String(), want)
				}
			}

			mu.Lock()
			defer mu.Unlock()
			if generatorCalls != len(specs) || startCalls != len(specs) ||
				requestCalls != len(specs) || stopCalls != len(specs) {
				t.Fatalf(
					"lifecycle calls: generator=%d start=%d requests=%d stop=%d, want %d each",
					generatorCalls, startCalls, requestCalls, stopCalls, len(specs),
				)
			}
			if len(observed) != len(issued) {
				t.Fatalf("observed expectations = %d, want %d", len(observed), len(issued))
			}
			seen := make(map[mockExpectations]bool, len(observed))
			for index, expectations := range observed {
				want := mockExpectations{
					apiKeySHA256:         sha256String(issued[index].apiKey),
					idempotencyKeySHA256: sha256String(issued[index].idempotencyKey),
				}
				if expectations != want {
					t.Fatalf("fingerprint expectations mismatch at index %d", index)
				}
				if seen[expectations] {
					t.Fatalf("expectations[%d] reuse an earlier fingerprint pair", index)
				}
				seen[expectations] = true
			}
			for _, secret := range issued {
				for _, forbidden := range []string{
					secret.apiKey,
					secret.idempotencyKey,
					sha256String(secret.apiKey),
					sha256String(secret.idempotencyKey),
				} {
					if strings.Contains(report.String(), forbidden) {
						t.Fatalf("runner report contains protected material")
					}
				}
			}
		})
	}
}

func TestRunExampleEnvironmentAllowlistAndTraceDirections(t *testing.T) {
	skipWindowsShellExecution(t)
	t.Setenv("CHAB_PROFILE", "poisoned")
	root := temporaryScriptRoot(t, "examples/ci/run.sh", `
[ -z "${CHAB_PROFILE:-}" ] || exit 9
"$CHAB_BIN" api get /credits --json >/dev/null
echo ok
`)
	r := realTestRunner()
	example := testExample(0, 1, []string{"ok"}, nil, []CommandUse{
		{Path: []string{"api", "get"}},
		{Path: []string{"credits", "balance"}, Flags: []string{"json"}},
	})
	got := joinErrors(r.runExample(root, t.TempDir(), 0, example))
	for _, want := range []string{
		`observed flag "--json" for command "api get" is not declared`,
		`declared command "credits balance" was not observed`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("errors missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "exit = 9") {
		t.Fatalf("developer environment reached script:\n%s", got)
	}
}

func TestRunExampleDetectsProtectedOutputChannelsAndForms(t *testing.T) {
	skipWindowsShellExecution(t)
	secrets := deterministicTestSecrets()
	protected, _ := newProtectedValues(secrets)
	var encodedURL string
	var encodedJSON string
	for _, form := range protected.forms {
		if encodedURL == "" && strings.Contains(form, "%22") {
			encodedURL = form
		}
		if encodedJSON == "" && strings.Contains(form, `\"`) {
			encodedJSON = form
		}
	}
	if encodedURL == "" || encodedJSON == "" {
		t.Fatalf("encoded protected forms are missing: %#v", protected.forms)
	}

	tests := []struct {
		name    string
		value   string
		channel string
		want    string
	}{
		{name: "api stdout", value: secrets.apiKey, channel: "stdout", want: "stdout contains a protected value"},
		{name: "api stderr", value: secrets.apiKey, channel: "stderr", want: "stderr contains a protected value"},
		{name: "idempotency stdout", value: secrets.idempotencyKey, channel: "stdout", want: "stdout contains a protected value"},
		{name: "idempotency stderr", value: secrets.idempotencyKey, channel: "stderr", want: "stderr contains a protected value"},
		{name: "URL-escaped stdout", value: encodedURL, channel: "stdout", want: "stdout contains a protected value"},
		{name: "JSON-escaped stderr", value: encodedJSON, channel: "stderr", want: "stderr contains a protected value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redirect := ""
			if tt.channel == "stderr" {
				redirect = " >&2"
			}
			shimBody := canonicalTraceShim("api", "get", "json") + "\n" +
				"printf '%s\\n' " + shellSingleQuoted(tt.value) + redirect
			root := temporaryScriptRoot(t, "examples/ci/run.sh", `
"$CHAB_BIN" api get /credits --json
echo ok
`)
			r := fakeBoundaryRunner(t, shimBody, nil)
			r.generateSecrets = func() (exampleSecrets, error) { return secrets, nil }
			example := testExample(0, 0, []string{"ok"}, nil, []CommandUse{{
				Path: []string{"api", "get"}, Flags: []string{"json"},
			}})
			got := joinErrors(r.runExample(root, t.TempDir(), 0, example))
			if !strings.Contains(got, tt.want) {
				t.Fatalf("errors missing %q:\n%s", tt.want, got)
			}
			if strings.Contains(got, tt.value) {
				t.Fatalf("diagnostics reproduced protected value:\n%s", got)
			}
		})
	}
}

func TestRunExampleRejectsMissingInvalidAndModeWrongTrace(t *testing.T) {
	skipWindowsShellExecution(t)
	tests := []struct {
		name     string
		shimBody string
		want     string
	}{
		{name: "missing", shimBody: "exit 0", want: "no such file"},
		{name: "invalid", shimBody: `
printf '%s\n' '{"path":[],"flags":[]}' > "$CHAB_EXAMPLE_TRACE_PATH"
chmod 0600 "$CHAB_EXAMPLE_TRACE_PATH"
`, want: "path must not be empty"},
		{name: "mode", shimBody: `
printf '%s\n' '{"path":["project","list"],"flags":["json"]}' > "$CHAB_EXAMPLE_TRACE_PATH"
chmod 0644 "$CHAB_EXAMPLE_TRACE_PATH"
`, want: "want 0600"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := temporaryScriptRoot(t, "examples/ci/run.sh", `"$CHAB_BIN" api get /credits --json; echo ok`)
			r := fakeBoundaryRunner(t, tt.shimBody, nil)
			example := testExample(0, 0, []string{"ok"}, nil, []CommandUse{{
				Path: []string{"api", "get"}, Flags: []string{"json"},
			}})
			got := joinErrors(r.runExample(root, t.TempDir(), 0, example))
			if !strings.Contains(got, tt.want) {
				t.Fatalf("errors missing %q:\n%s", tt.want, got)
			}
		})
	}
}

func TestProtectedValueShapesAndStreamLeakDetection(t *testing.T) {
	secrets := deterministicTestSecrets()
	protected, expectations := newProtectedValues(secrets)
	if len(expectations.apiKeySHA256) != 64 || len(expectations.idempotencyKeySHA256) != 64 {
		t.Fatalf("fingerprints = %#v", expectations)
	}
	raw := secrets.idempotencyKey
	query := ""
	jsonEscaped := ""
	for _, form := range protected.forms {
		switch {
		case form == raw:
		case strings.Contains(form, "%22"):
			query = form
		case strings.Contains(form, `\"`):
			jsonEscaped = form
		}
	}
	if query == "" || jsonEscaped == "" || raw == query || raw == jsonEscaped || query == jsonEscaped {
		t.Fatalf("idempotency representations are not distinct: raw=%q query=%q json=%q", raw, query, jsonEscaped)
	}

	requests := 0
	example := Example{Exit: 0, MockRequests: &requests}
	for _, test := range []struct {
		name   string
		stdout string
		stderr string
		want   string
	}{
		{name: "api stdout", stdout: secrets.apiKey, want: "stdout contains a protected value"},
		{name: "api stderr", stderr: secrets.apiKey, want: "stderr contains a protected value"},
		{name: "idempotency stdout", stdout: secrets.idempotencyKey, want: "stdout contains a protected value"},
		{name: "idempotency stderr", stderr: secrets.idempotencyKey, want: "stderr contains a protected value"},
		{name: "encoded stdout", stdout: query, want: "stdout contains a protected value"},
		{name: "json stderr", stderr: jsonEscaped, want: "stderr contains a protected value"},
		{name: "fingerprint stdout", stdout: expectations.apiKeySHA256, want: "stdout contains a protected value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := joinErrors(assertOutcome(example, 0, test.stdout, test.stderr, &requests, protected))
			if !strings.Contains(got, test.want) {
				t.Fatalf("errors missing %q:\n%s", test.want, got)
			}
		})
	}
}

func TestRunExampleScansLogsAfterStopOnSuccess(t *testing.T) {
	skipWindowsShellExecution(t)
	secrets := deterministicTestSecrets()
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "api key", value: secrets.apiKey},
		{name: "idempotency key", value: secrets.idempotencyKey},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := temporaryScriptRoot(t, "examples/ci/run.sh", `"$CHAB_BIN" api get /credits --json; echo ok`)
			r := fakeBoundaryRunner(t, canonicalTraceShim("api", "get", "json"), func(logs io.Writer, expectations mockExpectations) mockHandle {
				return mockHandle{
					baseURL: "http://example.test",
					requests: func() (int, error) {
						return 0, nil
					},
					stop: func() error {
						_, _ = fmt.Fprintln(logs, test.value)
						return nil
					},
				}
			})
			r.generateSecrets = func() (exampleSecrets, error) { return secrets, nil }
			example := testExample(0, 0, []string{"ok"}, nil, []CommandUse{{
				Path: []string{"api", "get"}, Flags: []string{"json"},
			}})
			got := joinErrors(r.runExample(root, t.TempDir(), 0, example))
			if !strings.Contains(got, "mock log contains a protected value") {
				t.Fatalf("errors missing stopped-log leak:\n%s", got)
			}
			if strings.Contains(got, test.value) {
				t.Fatalf("diagnostics leaked protected value:\n%s", got)
			}
		})
	}
}

func TestAssertOutcomeReportsAssertionsCountersAndShimFailure(t *testing.T) {
	protected, _ := newProtectedValues(deterministicTestSecrets())
	wantRequests := 2
	example := Example{
		Exit:           0,
		StdoutContains: []string{"stdout needle"},
		StderrContains: []string{"stderr needle"},
		MockRequests:   &wantRequests,
	}
	got := joinErrors(assertOutcome(example, shimFailureExit, "other", "other", nil, protected))
	for _, want := range []string{
		"invocation shim failed",
		`stdout missing "stdout needle"`,
		`stderr missing "stderr needle"`,
		"mock request count unavailable, want 2",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("errors missing %q:\n%s", want, got)
		}
	}
	got = joinErrors(assertOutcome(example, 0, "stdout needle", "stderr needle", intPtr(1), protected))
	if !strings.Contains(got, "mock_requests = 1, want 2") {
		t.Fatalf("counter mismatch errors:\n%s", got)
	}
}

func TestRunExampleRejectsSubstitutedCredentialAndIdempotency(t *testing.T) {
	skipWindowsShellExecution(t)
	tests := []struct {
		name      string
		body      string
		errorCode string
		requests  int
		commands  []CommandUse
	}{
		{
			name:      "credential",
			body:      `CHAB_API_KEY=substituted-value "$CHAB_BIN" api get /credits --json`,
			errorCode: `"code":"invalid_api_token"`,
			requests:  1,
			commands:  []CommandUse{{Path: []string{"api", "get"}, Flags: []string{"json"}}},
		},
		{
			name:      "idempotency",
			body:      `"$CHAB_BIN" api post /projects --field name=Rejected --idempotency-key substituted-value --json --no-prompt`,
			errorCode: `"code":"idempotency_key_conflict"`,
			requests:  2,
			commands: []CommandUse{{
				Path: []string{"api", "post"}, Flags: []string{"field", "idempotency-key", "json", "no-prompt"},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := temporaryScriptRoot(t, "examples/ci/run.sh", tt.body)
			r := realTestRunner()
			example := testExample(0, tt.requests, nil, []string{tt.errorCode}, tt.commands)
			got := joinErrors(r.runExample(root, t.TempDir(), 0, example))
			if got == "" {
				t.Fatal("substituted value unexpectedly succeeded")
			}
			for _, want := range []string{
				"exit = ",
				", want 0",
				tt.errorCode,
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("diagnostics missing %q:\n%s", want, got)
				}
			}
			for _, unrelated := range []string{
				"stderr missing",
				"mock_requests =",
				"mock request count",
				"invocation trace",
				"observed command",
				"declared command",
				"observed flag",
				"declared flag",
				"invocation shim failed",
				"script timed out",
				"checker diagnostic",
			} {
				if strings.Contains(got, unrelated) {
					t.Fatalf("diagnostics contain unrelated failure %q:\n%s", unrelated, got)
				}
			}
			secrets := deterministicTestSecrets()
			_, expectations := newProtectedValues(secrets)
			for _, forbidden := range []string{
				"substituted-value",
				secrets.apiKey,
				secrets.idempotencyKey,
				expectations.apiKeySHA256,
				expectations.idempotencyKeySHA256,
			} {
				if strings.Contains(got, forbidden) {
					t.Fatalf("diagnostics leaked %q:\n%s", forbidden, got)
				}
			}
		})
	}
}

func TestStartedMockRequestCounterBoundary(t *testing.T) {
	t.Setenv("CHAB_MOCK_EXPECTED_API_KEY_SHA256", strings.Repeat("0", 64))
	t.Setenv("CHAB_MOCK_EXPECTED_IDEMPOTENCY_KEY_SHA256", strings.Repeat("f", 64))
	r := realTestRunner()
	secrets := deterministicTestSecrets()
	_, expectations := newProtectedValues(secrets)
	var logs bytes.Buffer
	handle, err := r.startMockProcess(&logs, expectations)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := handle.stop(); err != nil {
			t.Errorf("stop mock: %v", err)
		}
	}()
	if count, err := handle.requests(); err != nil || count != 0 {
		t.Fatalf("initial count=%d err=%v", count, err)
	}
	stateDir := t.TempDir()
	command := exec.Command(testViltBin, "api", "get", "/projects", "--json")
	command.Env = []string{
		"HOME=" + stateDir,
		"LC_ALL=C",
		"CHAB_CONFIG=" + filepath.Join(stateDir, "config.yml"),
		"CHAB_AUTH_FILE=" + filepath.Join(stateDir, "auth.json"),
		"CHAB_API_KEY=" + secrets.apiKey,
		"CHAB_BASE_URL=" + handle.baseURL,
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("chab request: %v\n%s", err, output)
	}
	if count, err := handle.requests(); err != nil || count != 1 {
		t.Fatalf("final count=%d err=%v", count, err)
	}
}

func TestRunExampleTerminatesBackgroundDescendantBeforeMockCallbacks(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux process-group assertion")
	}
	root := temporaryScriptRoot(t, "examples/ci/run.sh", `
"$CHAB_BIN" api get /credits --json
sleep 30 &
echo "$!" > "$HOME/descendant.pid"
wait
`)
	tmp := t.TempDir()
	pidPath := filepath.Join(tmp, "state", "001", "descendant.pid")
	var mu sync.Mutex
	var order []string
	assertGone := func(stage string) error {
		data, err := os.ReadFile(pidPath)
		if err != nil {
			return err
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid))); err == nil {
			return fmt.Errorf("descendant still exists before %s", stage)
		}
		mu.Lock()
		order = append(order, stage)
		mu.Unlock()
		return nil
	}
	r := fakeBoundaryRunner(t, canonicalTraceShim("api", "get", "json"), func(_ io.Writer, _ mockExpectations) mockHandle {
		return mockHandle{
			baseURL: "http://example.test",
			requests: func() (int, error) {
				return 0, assertGone("requests")
			},
			stop: func() error {
				return assertGone("stop")
			},
		}
	})
	r.exampleTimeout = 250 * time.Millisecond
	example := testExample(0, 0, nil, []string{"never"}, []CommandUse{{
		Path: []string{"api", "get"}, Flags: []string{"json"},
	}})
	got := joinErrors(r.runExample(root, tmp, 0, example))
	if !strings.Contains(got, "script timed out") {
		t.Fatalf("errors missing timeout:\n%s", got)
	}
	if strings.Contains(got, "descendant still exists") {
		t.Fatalf("descendant cleanup ordering failed:\n%s", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(order, ",") != "requests,stop" {
		t.Fatalf("callback order = %v", order)
	}
}

func TestRunExampleCancellationTerminatesBackgroundDescendantBeforeMockCallbacks(t *testing.T) {
	skipWindowsShellExecution(t)
	root := temporaryScriptRoot(t, "examples/ci/run.sh", `
(
  trap '' TERM
  while :; do
    sleep 1
  done
) &
echo "$!" > "$HOME/descendant.pid"
wait
`)
	tmp := t.TempDir()
	pidPath := filepath.Join(tmp, "state", "001", "descendant.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var order []string
	assertGone := func(stage string) error {
		data, err := os.ReadFile(pidPath)
		if err != nil {
			return err
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid))); err == nil {
			return fmt.Errorf("descendant still exists before %s", stage)
		}
		mu.Lock()
		order = append(order, stage)
		mu.Unlock()
		return nil
	}
	r := fakeBoundaryRunner(t, canonicalTraceShim("api", "get", "json"), func(_ io.Writer, _ mockExpectations) mockHandle {
		return mockHandle{
			baseURL: "http://example.test",
			requests: func() (int, error) {
				return 0, assertGone("requests")
			},
			stop: func() error {
				return assertGone("stop")
			},
		}
	})
	r.opts.Context = ctx
	example := testExample(0, 0, nil, []string{"never"}, []CommandUse{{
		Path: []string{"api", "get"}, Flags: []string{"json"},
	}})

	cancelDone := make(chan struct{})
	go func() {
		defer close(cancelDone)
		defer cancel()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			// Opening the redirection creates an empty file before printf writes
			// the PID. Do not cancel the shell in that intermediate state.
			if data, err := os.ReadFile(pidPath); err == nil {
				if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
					return
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	got := joinErrors(r.runExample(root, tmp, 0, example))
	<-cancelDone
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("context error = %v, want canceled", ctx.Err())
	}
	if strings.Contains(got, "descendant still exists") {
		t.Fatalf("descendant cleanup ordering failed:\n%s", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(order, ",") != "requests,stop" {
		t.Fatalf("callback order = %v", order)
	}
}

func TestBuildBinaryHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tmp := t.TempDir()
	err := buildBinary(ctx, tmp, filepath.Join(tmp, "go-build"), filepath.Join(tmp, "chab"), "./cmd/chab")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("build error = %v, want context canceled", err)
	}
}

func TestBuildBinaryUsesOwnedTemporaryDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", []byte("module example.test/build\n\ngo 1.22\n"), 0o644)
	writeTestFile(t, root, "main.go", []byte("package main\nfunc main() {}\n"), 0o644)
	t.Setenv("GOTMPDIR", filepath.Join(root, "ambient-missing"))
	buildTmp := filepath.Join(root, "owned-build-temp")
	out := filepath.Join(root, exeName("example"))
	if err := buildBinary(context.Background(), root, buildTmp, out, "."); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("built binary: %v", err)
	}
	if info, err := os.Stat(buildTmp); err != nil || !info.IsDir() {
		t.Fatalf("owned build temp was not created as a directory: info=%v err=%v", info, err)
	}
}

func TestBuildBinaryCancellationTerminatesBackgroundDescendant(t *testing.T) {
	skipWindowsShellExecution(t)
	root := t.TempDir()
	pidPath := filepath.Join(root, "descendant.pid")
	fakeBin := filepath.Join(root, "bin")
	body := fmt.Sprintf(`#!/bin/sh
set -eu
(
  trap '' TERM
  while :; do
    sleep 1
  done
) &
printf '%%s\n' "$!" > %s
wait
`, shellSingleQuoted(pidPath))
	writeTestFile(t, fakeBin, "go", []byte(body), 0o755)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelDone := make(chan struct{})
	go func() {
		defer close(cancelDone)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(pidPath); err == nil {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	err := buildBinary(ctx, root, filepath.Join(root, "go-build"), filepath.Join(root, "out"), ".")
	<-cancelDone
	if err == nil || !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("build error = %v, context error = %v", err, ctx.Err())
	}
	data, readErr := os.ReadFile(pidPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if _, statErr := os.Stat(filepath.Join("/proc", strconv.Itoa(pid))); statErr == nil {
		t.Fatalf("build descendant %d still exists after cancellation", pid)
	}
}

func TestGeneratedIdempotencyKeyHasVisibleDistinctEncodingShape(t *testing.T) {
	secrets, err := generateExampleSecrets()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(secrets.idempotencyKey, `"`) != 1 {
		t.Fatalf("idempotency key quote count = %d", strings.Count(secrets.idempotencyKey, `"`))
	}
	if len(secrets.idempotencyKey) < 1 || len(secrets.idempotencyKey) > 255 {
		t.Fatalf("idempotency key length = %d, want 1..255", len(secrets.idempotencyKey))
	}
	for _, r := range secrets.idempotencyKey {
		if r < 0x21 || r > 0x7e {
			t.Fatalf("idempotency key contains non-visible ASCII %U", r)
		}
	}
}

func realTestRunner() *runner {
	r := &runner{
		viltBin: testViltBin,
		mockBin: testMockBin,
		shimBin: testShimBin,
		generateSecrets: func() (exampleSecrets, error) {
			return deterministicTestSecrets(), nil
		},
	}
	r.startMock = r.startMockProcess
	return r
}

// fakeBoundaryRunner replaces the forwarding shim and mock lifecycle while
// retaining the real runner's environment, trace, outcome, and leak checks.
func fakeBoundaryRunner(t *testing.T, shimBody string, starter func(io.Writer, mockExpectations) mockHandle) *runner {
	t.Helper()
	shim := filepath.Join(t.TempDir(), "shim")
	if err := os.WriteFile(shim, []byte("#!/bin/sh\nset -eu\n"+strings.TrimSpace(shimBody)+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := &runner{
		viltBin: testViltBin,
		mockBin: testMockBin,
		shimBin: shim,
		generateSecrets: func() (exampleSecrets, error) {
			return deterministicTestSecrets(), nil
		},
	}
	r.startMock = func(logs io.Writer, expectations mockExpectations) (mockHandle, error) {
		if starter != nil {
			return starter(logs, expectations), nil
		}
		return mockHandle{
			baseURL:  "http://example.test",
			requests: func() (int, error) { return 0, nil },
			stop:     func() error { return nil },
		}, nil
	}
	return r
}

// skipWindowsShellExecution keeps POSIX-shell runner tests off Windows while
// pure validation and direct CLI/mock process tests continue to run there.
func skipWindowsShellExecution(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("example runner tests use POSIX sh")
	}
}

// canonicalTraceShim returns a small shell shim that writes one valid trace
// record. Tests append only the behavior needed for the boundary they exercise.
func canonicalTraceShim(path ...string) string {
	flags := path[len(path)-1]
	elements := path[:len(path)-1]
	quoted := make([]string, 0, len(elements))
	for _, element := range elements {
		quoted = append(quoted, `"`+element+`"`)
	}
	return fmt.Sprintf(
		`printf '%%s\n' '{"path":[%s],"flags":["%s"]}' >> "$CHAB_EXAMPLE_TRACE_PATH"
chmod 0600 "$CHAB_EXAMPLE_TRACE_PATH"`,
		strings.Join(quoted, ","),
		flags,
	)
}

func shellSingleQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func temporaryScriptRoot(t *testing.T, rel, body string) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, root, rel, []byte("#!/bin/sh\nset -eu\n"+strings.TrimSpace(body)+"\n"), 0o755)
	return root
}

func testExample(exit, requests int, stdoutContains, stderrContains []string, commands []CommandUse) Example {
	return Example{
		Script:         "examples/ci/run.sh",
		Exit:           exit,
		StdoutContains: stdoutContains,
		StderrContains: stderrContains,
		MockRequests:   &requests,
		Commands:       commands,
	}
}

func intPtr(value int) *int {
	return &value
}

func deterministicTestSecrets() exampleSecrets {
	return exampleSecrets{
		apiKey:         "runner-api-key-for-tests",
		idempotencyKey: `runner-idempotency-key-for-tests"`,
	}
}

func repoRootFromWorkingDirectory() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate repository root")
		}
		dir = parent
	}
}
