package examplecheck

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/redact"
)

const (
	manifestPath    = "examples/check-examples.json"
	scriptTimeout   = 60 * time.Second
	mockTimeout     = 5 * time.Second
	shimFailureExit = 125
)

// RunOptions configures the example checker command.
type RunOptions struct {
	RepoRoot string
	Stderr   io.Writer
	Context  context.Context
}

// exampleSecrets contains the fresh values issued to one script run.
type exampleSecrets struct {
	apiKey         string
	idempotencyKey string
}

// mockExpectations contains only fingerprints safe to pass to the verifying
// mock process.
type mockExpectations struct {
	apiKeySHA256         string
	idempotencyKeySHA256 string
}

// runner owns one checker invocation. Tests can inject binaries, deterministic
// secrets, or a mock starter; production builds fresh binaries.
type runner struct {
	opts            RunOptions
	viltBin         string
	mockBin         string
	shimBin         string
	exampleTimeout  time.Duration
	generateSecrets func() (exampleSecrets, error)
	startMock       func(io.Writer, mockExpectations) (mockHandle, error)
}

// mockHandle is the small control surface the checker needs from either the
// real mock process or a test double.
type mockHandle struct {
	baseURL  string
	requests func() (int, error)
	stop     func() error
}

// protectedValues keeps exact-redaction state separate from the broader set of
// raw and encoded forms that must cause a checker failure.
type protectedValues struct {
	registry *redact.Registry
	forms    []string
}

// Run validates the manifest and runs every non-live example script.
func Run(opts RunOptions) error {
	r := &runner{opts: opts}
	return r.run()
}

// run performs the full checker workflow: static manifest validation, binary
// builds, isolated script execution, and final reporting.
func (r *runner) run() error {
	if r.opts.RepoRoot == "" {
		r.opts.RepoRoot = "."
	}
	if r.opts.Stderr == nil {
		r.opts.Stderr = io.Discard
	}
	if r.opts.Context == nil {
		r.opts.Context = context.Background()
	}
	repoRoot, err := filepath.Abs(r.opts.RepoRoot)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err != nil {
		return fmt.Errorf("run from the repository root: %w", err)
	}

	manifest, staticErrs, err := committedStaticChecks(repoRoot)
	if err != nil {
		return err
	}
	if len(staticErrs) > 0 {
		for _, staticErr := range staticErrs {
			fmt.Fprintln(r.opts.Stderr, staticErr)
		}
		return fmt.Errorf("example static checks failed with %d problem(s)", len(staticErrs))
	}

	tmp, err := os.MkdirTemp("", "chab-examplecheck-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	// Build each component at most once per checker invocation. Tests can inject
	// any component independently without changing the production lifecycle.
	if r.viltBin == "" || r.mockBin == "" || r.shimBin == "" {
		binDir := filepath.Join(tmp, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return err
		}
		buildTmp := filepath.Join(tmp, "go-build")
		if r.viltBin == "" {
			r.viltBin = filepath.Join(binDir, exeName("chab"))
			if err := buildBinary(r.opts.Context, repoRoot, buildTmp, r.viltBin, "./cmd/chab"); err != nil {
				if contextErr := r.opts.Context.Err(); contextErr != nil {
					return fmt.Errorf("example checks canceled: %w", contextErr)
				}
				return err
			}
		}
		if r.mockBin == "" {
			r.mockBin = filepath.Join(binDir, exeName("mockapi"))
			if err := buildBinary(r.opts.Context, repoRoot, buildTmp, r.mockBin, "./examples/mockapi"); err != nil {
				if contextErr := r.opts.Context.Err(); contextErr != nil {
					return fmt.Errorf("example checks canceled: %w", contextErr)
				}
				return err
			}
		}
		if r.shimBin == "" {
			r.shimBin = filepath.Join(binDir, exeName("chab-example-shim"))
			if err := buildBinary(r.opts.Context, repoRoot, buildTmp, r.shimBin, "./scripts/example-invocation-shim"); err != nil {
				if contextErr := r.opts.Context.Err(); contextErr != nil {
					return fmt.Errorf("example checks canceled: %w", contextErr)
				}
				return err
			}
		}
	}
	if r.generateSecrets == nil {
		r.generateSecrets = generateExampleSecrets
	}
	if r.startMock == nil {
		r.startMock = r.startMockProcess
	}

	failures := 0
	for index, example := range manifest.Examples {
		errs := r.runExample(repoRoot, tmp, index, example)
		if contextErr := r.opts.Context.Err(); contextErr != nil {
			return fmt.Errorf("example checks canceled: %w", contextErr)
		}
		if len(errs) > 0 {
			failures++
			fmt.Fprintf(r.opts.Stderr, "fail %s\n", example.Script)
			for _, exampleErr := range errs {
				fmt.Fprintf(r.opts.Stderr, "  %v\n", exampleErr)
			}
			continue
		}
		fmt.Fprintf(r.opts.Stderr, "ok %s\n", example.Script)
	}
	if failures > 0 {
		return fmt.Errorf("example checks failed for %d script(s)", failures)
	}
	return nil
}

// committedStaticChecks contains the validations that also protect committed
// examples in regular go test runs.
func committedStaticChecks(repoRoot string) (Manifest, []error, error) {
	manifest, err := LoadManifest(filepath.Join(repoRoot, filepath.FromSlash(manifestPath)))
	if err != nil {
		return Manifest{}, nil, err
	}
	discovered, err := DiscoverScripts(repoRoot)
	if err != nil {
		return Manifest{}, nil, err
	}
	var errs []error
	errs = append(errs, ValidateStructure(manifest)...)
	errs = append(errs, ValidateLayout(manifest, discovered)...)
	errs = append(errs, committedScriptModeErrors(repoRoot, discovered, runtime.GOOS)...)
	errs = append(errs, ValidateScriptLineCounts(repoRoot, discovered)...)
	errs = append(errs, ValidateSafety(repoRoot, manifestPath, discovered)...)
	root := cli.NewRootCommand(io.Discard, io.Discard)
	errs = append(errs, ValidateDrift(manifest, cli.VisibleCommands(root, false), cli.Catalog())...)
	return manifest, errs, nil
}

// committedScriptModeErrors keeps executable-bit enforcement on the Linux
// checked-workflow gate while other platforms retain the portable checks.
func committedScriptModeErrors(repoRoot string, paths []string, goos string) []error {
	if goos != "linux" {
		return nil
	}
	return ValidateScriptModes(repoRoot, paths)
}

// runExample starts a fresh verifying mock, runs one shell script with isolated
// state, stops every process, and then validates output, trace, and logs.
func (r *runner) runExample(repoRoot, tmp string, index int, example Example) []error {
	stateDir := filepath.Join(tmp, "state", fmt.Sprintf("%03d", index+1))
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return []error{err}
	}
	tracePath := filepath.Join(stateDir, "invocations.ndjson")

	secrets, err := r.generateSecrets()
	if err != nil {
		return []error{fmt.Errorf("generate example secrets: %w", err)}
	}
	protected, expectations := newProtectedValues(secrets)

	var mockLogs bytes.Buffer
	handle, err := r.startMock(&mockLogs, expectations)
	if err != nil {
		errs := []error{fmt.Errorf("start mock: %w", err)}
		if containsProtected(mockLogs.Bytes(), protected.forms) {
			errs = append(errs, fmt.Errorf("mock log contains a protected value"))
		}
		if mockLogs.Len() > 0 {
			errs = append(errs, fmt.Errorf("mock stderr excerpt:\n%s", safeExcerpt(tail(mockLogs.String(), 60), protected)))
		}
		return finalizeDiagnostics(errs, protected)
	}

	timeout := r.exampleTimeout
	if timeout <= 0 {
		timeout = scriptTimeout
	}
	parentContext := r.opts.Context
	if parentContext == nil {
		parentContext = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentContext, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, "sh", filepath.Join(repoRoot, filepath.FromSlash(example.Script)))
	command.Dir = repoRoot
	// The explicit allowlist prevents developer credentials, profiles, proxies,
	// and unrelated Chab settings from affecting checked examples.
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + stateDir,
		"LC_ALL=C",
		"CHAB_BIN=" + r.shimBin,
		"CHAB_EXAMPLE_REAL_BIN=" + r.viltBin,
		"CHAB_EXAMPLE_TRACE_PATH=" + tracePath,
		"CHAB_EXAMPLE_IDEMPOTENCY_KEY=" + secrets.idempotencyKey,
		"CHAB_CONFIG=" + filepath.Join(stateDir, "config.yml"),
		"CHAB_AUTH_FILE=" + filepath.Join(stateDir, "auth.json"),
		"CHAB_API_KEY=" + secrets.apiKey,
		"CHAB_BASE_URL=" + handle.baseURL,
	}
	prepareProcessCommand(command)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	exitCode := exitCodeFromError(runErr)

	var errs []error
	// Command.Run waits for the shell, while this platform helper also proves
	// that background descendants are gone before any outcome is collected.
	if finishErr := finishProcessCommand(command); finishErr != nil {
		errs = append(errs, fmt.Errorf("terminate script process group: %w", finishErr))
	}
	if ctx.Err() == context.DeadlineExceeded {
		errs = append(errs, fmt.Errorf("script timed out"))
	} else if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			errs = append(errs, fmt.Errorf("script execution failed: %w", runErr))
		}
	}

	// The counter is available only while the mock is running. Stopping it then
	// waits for process and stderr-copy completion before trace and log scans.
	var mockRequests *int
	count, countErr := handle.requests()
	if countErr != nil {
		errs = append(errs, fmt.Errorf("mock request count failed: %w", countErr))
	} else {
		mockRequests = &count
	}
	if stopErr := handle.stop(); stopErr != nil {
		errs = append(errs, fmt.Errorf("stop mock: %w", stopErr))
	}

	observed, traceRaw, traceErr := loadTrace(tracePath)
	if traceErr != nil {
		errs = append(errs, traceErr)
	} else {
		errs = append(errs, compareTrace(example, observed)...)
	}
	errs = append(errs, assertOutcome(example, exitCode, stdout.String(), stderr.String(), mockRequests, protected)...)
	if containsProtected(traceRaw, protected.forms) {
		errs = append(errs, fmt.Errorf("invocation trace contains a protected value"))
	}
	if traceErr != nil && containsProtected([]byte(traceErr.Error()), protected.forms) {
		errs = append(errs, fmt.Errorf("invocation trace diagnostic contains a protected value"))
	}
	if containsProtected(mockLogs.Bytes(), protected.forms) {
		errs = append(errs, fmt.Errorf("mock log contains a protected value"))
	}

	if len(errs) > 0 {
		errs = append(errs,
			fmt.Errorf("stdout excerpt:\n%s", safeExcerpt(stdout.String(), protected)),
			fmt.Errorf("stderr excerpt:\n%s", safeExcerpt(stderr.String(), protected)),
			fmt.Errorf("mock stderr excerpt:\n%s", safeExcerpt(tail(mockLogs.String(), 60), protected)),
		)
	}
	return finalizeDiagnostics(errs, protected)
}

// assertOutcome compares streams and counters and scans both child channels for
// every protected raw, encoded, escaped, and fingerprint representation.
func assertOutcome(e Example, exit int, stdout, stderr string, mockRequests *int, protected protectedValues) []error {
	var errs []error
	if exit == shimFailureExit || strings.Contains(stderr, "chab example shim: ") {
		errs = append(errs, fmt.Errorf("invocation shim failed"))
	} else if exit != e.Exit {
		errs = append(errs, fmt.Errorf("exit = %d, want %d", exit, e.Exit))
	}
	for _, want := range e.StdoutContains {
		if !strings.Contains(stdout, want) {
			errs = append(errs, fmt.Errorf("stdout missing %q", want))
		}
	}
	for _, want := range e.StderrContains {
		if !strings.Contains(stderr, want) {
			errs = append(errs, fmt.Errorf("stderr missing %q", want))
		}
	}
	if e.MockRequests != nil {
		if mockRequests == nil {
			errs = append(errs, fmt.Errorf("mock request count unavailable, want %d", *e.MockRequests))
		} else if *mockRequests != *e.MockRequests {
			errs = append(errs, fmt.Errorf("mock_requests = %d, want %d", *mockRequests, *e.MockRequests))
		}
	}
	if containsProtected([]byte(stdout), protected.forms) {
		errs = append(errs, fmt.Errorf("stdout contains a protected value"))
	}
	if containsProtected([]byte(stderr), protected.forms) {
		errs = append(errs, fmt.Errorf("stderr contains a protected value"))
	}
	return errs
}

// newProtectedValues derives the mock's expected fingerprints and every
// representation that must be absent from outputs, traces, logs, and
// diagnostics. Encoded forms are checked explicitly because exact redaction
// alone cannot recognize a transformed secret.
func newProtectedValues(secrets exampleSecrets) (protectedValues, mockExpectations) {
	apiFingerprint := sha256String(secrets.apiKey)
	idempotencyFingerprint := sha256String(secrets.idempotencyKey)
	registry := redact.NewRegistry()
	for _, value := range []string{
		secrets.apiKey,
		secrets.idempotencyKey,
		apiFingerprint,
		idempotencyFingerprint,
	} {
		registry.RegisterSecret(value)
	}

	seen := make(map[string]struct{})
	var forms []string
	add := func(value string) {
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		forms = append(forms, value)
	}
	for _, value := range []string{secrets.apiKey, secrets.idempotencyKey} {
		add(value)
		add(url.QueryEscape(value))
		add(url.PathEscape(value))
		if encoded, err := json.Marshal(value); err == nil && len(encoded) >= 2 {
			add(string(encoded[1 : len(encoded)-1]))
		}
	}
	add(apiFingerprint)
	add(idempotencyFingerprint)
	return protectedValues{registry: registry, forms: forms}, mockExpectations{
		apiKeySHA256:         apiFingerprint,
		idempotencyKeySHA256: idempotencyFingerprint,
	}
}

func containsProtected(text []byte, forms []string) bool {
	for _, form := range forms {
		if form != "" && bytes.Contains(text, []byte(form)) {
			return true
		}
	}
	return false
}

// safeExcerpt redacts known exact values and withholds the whole excerpt if an
// encoded representation still survives.
func safeExcerpt(text string, protected protectedValues) string {
	redacted := protected.registry.RedactText([]byte(tail(text, 60)))
	if containsProtected(redacted, protected.forms) {
		return "[protected content omitted]\n"
	}
	return string(redacted)
}

// finalizeDiagnostics is the final fail-closed boundary before an error can be
// printed. One generic error replaces any diagnostic that still carries a
// protected representation.
func finalizeDiagnostics(errs []error, protected protectedValues) []error {
	if len(errs) == 0 {
		return nil
	}
	safe := make([]error, 0, len(errs))
	checkerLeakReported := false
	for _, err := range errs {
		if err == nil {
			continue
		}
		raw := []byte(err.Error())
		if containsProtected(raw, protected.forms) {
			if !checkerLeakReported {
				safe = append(safe, fmt.Errorf("checker diagnostic contains a protected value"))
				checkerLeakReported = true
			}
			continue
		}
		redacted := protected.registry.RedactText(raw)
		if containsProtected(redacted, protected.forms) {
			if !checkerLeakReported {
				safe = append(safe, fmt.Errorf("checker diagnostic contains a protected value"))
				checkerLeakReported = true
			}
			continue
		}
		safe = append(safe, errors.New(string(redacted)))
	}
	return safe
}

// startMockProcess launches the compiled mock and reads its one-line stdout
// startup protocol before returning a usable base URL.
func (r *runner) startMockProcess(stderr io.Writer, expectations mockExpectations) (mockHandle, error) {
	command := exec.Command(r.mockBin, "--listen", "127.0.0.1:0")
	// Do not inherit the developer environment. The mock needs only the two
	// expected fingerprints and must not receive the underlying secret values.
	command.Env = []string{
		"CHAB_MOCK_EXPECTED_API_KEY_SHA256=" + expectations.apiKeySHA256,
		"CHAB_MOCK_EXPECTED_IDEMPOTENCY_KEY_SHA256=" + expectations.idempotencyKeySHA256,
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return mockHandle{}, err
	}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return mockHandle{}, err
	}

	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		// The mock reserves stdout for exactly one LISTEN_ADDR line. Additional
		// process logs go to stderr and are collected separately.
		reader := bufio.NewReader(stdout)
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			errCh <- readErr
			return
		}
		lineCh <- line
	}()

	var line string
	select {
	case line = <-lineCh:
	case readErr := <-errCh:
		killAndWait(command)
		return mockHandle{}, readErr
	case <-time.After(mockTimeout):
		killAndWait(command)
		return mockHandle{}, fmt.Errorf("mock did not announce LISTEN_ADDR within %s", mockTimeout)
	}
	if !strings.HasPrefix(line, "LISTEN_ADDR=") {
		killAndWait(command)
		return mockHandle{}, fmt.Errorf("mock announced an invalid startup line")
	}
	addr := strings.TrimSpace(strings.TrimPrefix(line, "LISTEN_ADDR="))
	baseURL := "http://" + addr
	if err := waitReady(baseURL); err != nil {
		killAndWait(command)
		return mockHandle{}, err
	}

	var stopOnce sync.Once
	var stopErr error
	// stop is idempotent and joins the process so the stderr copy has completed
	// before callers inspect the collected mock log.
	stop := func() error {
		stopOnce.Do(func() {
			if command.Process != nil {
				if err := stopMockProcess(command.Process); err != nil && !errors.Is(err, os.ErrProcessDone) {
					stopErr = err
				}
			}
			done := make(chan error, 1)
			go func() {
				done <- command.Wait()
			}()
			select {
			case waitErr := <-done:
				if waitErr != nil && stopErr == nil && !expectedMockStopWaitError(waitErr) {
					stopErr = waitErr
				}
			case <-time.After(2 * time.Second):
				if command.Process != nil {
					_ = command.Process.Kill()
				}
				waitErr := <-done
				if waitErr != nil && stopErr == nil && !expectedMockStopWaitError(waitErr) {
					stopErr = waitErr
				}
			}
		})
		return stopErr
	}
	return mockHandle{
		baseURL: baseURL,
		requests: func() (int, error) {
			return mockRequestCount(baseURL)
		},
		stop: stop,
	}, nil
}

// killAndWait closes an incomplete startup path without leaving either the
// process or os/exec's output-copy goroutines behind.
func killAndWait(command *exec.Cmd) {
	if command.Process != nil {
		_ = command.Process.Kill()
	}
	_ = command.Wait()
}

// waitReady polls the mock health endpoint after LISTEN_ADDR so the script does
// not race the HTTP server startup.
func waitReady(baseURL string) error {
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(mockTimeout)
	for time.Now().Before(deadline) {
		response, err := client.Get(baseURL + "/readyz")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("mock did not become ready within %s", mockTimeout)
}

// mockRequestCount reads the mock's admin counter used by manifest assertions.
func mockRequestCount(baseURL string) (int, error) {
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get(baseURL + "/__mock/requests")
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("mock request-count status %d", response.StatusCode)
	}
	var body struct {
		APIRequests int `json:"api_requests"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return 0, err
	}
	return body.APIRequests, nil
}

// buildBinary captures compiler output and adds package context when checker
// setup fails.
func buildBinary(ctx context.Context, repoRoot, buildTmp, out, pkg string) error {
	if err := os.MkdirAll(buildTmp, 0o700); err != nil {
		return fmt.Errorf("prepare build temp for %s: %w", pkg, err)
	}
	command := exec.CommandContext(ctx, "go", "build", "-o", out, pkg)
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "GOTMPDIR="+buildTmp)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	prepareProcessCommand(command)
	runErr := command.Run()
	finishErr := finishProcessCommand(command)
	if runErr != nil {
		if finishErr != nil {
			return fmt.Errorf("build %s: %w; process cleanup: %v\n%s", pkg, runErr, finishErr, output.String())
		}
		return fmt.Errorf("build %s: %w\n%s", pkg, runErr, output.String())
	}
	if finishErr != nil {
		return fmt.Errorf("finish build %s processes: %w", pkg, finishErr)
	}
	return nil
}

// generateExampleSecrets creates independent values for one workflow. The
// idempotency key includes one quote so URL and JSON leak forms are distinct
// and therefore exercised by the checker.
func generateExampleSecrets() (exampleSecrets, error) {
	apiEntropy := make([]byte, 16)
	if _, err := rand.Read(apiEntropy); err != nil {
		return exampleSecrets{}, err
	}
	idempotencyEntropy := make([]byte, 16)
	if _, err := rand.Read(idempotencyEntropy); err != nil {
		return exampleSecrets{}, err
	}
	return exampleSecrets{
		apiKey:         "mock-key-" + hex.EncodeToString(apiEntropy),
		idempotencyKey: `mock-idem-` + hex.EncodeToString(idempotencyEntropy) + `"`,
	}, nil
}

func sha256String(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func tail(text string, maxLines int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) <= maxLines {
		return text
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n") + "\n"
}
