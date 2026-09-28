// Package installcheck exercises scripts/install.sh as an external POSIX sh
// process against a local release server.
package installcheck

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	fixtureVersion = "0.6.0"
	fixtureBinary  = "#!/bin/sh\necho fixture\n"
	existingBinary = "#!/bin/sh\necho existing\n"
)

type scriptResult struct {
	stdout   string
	stderr   string
	exitCode int
}

type releaseFixture struct {
	server   *httptest.Server
	requests atomic.Int64
}

type tarEntry struct {
	name     string
	data     []byte
	mode     int64
	typeflag byte
	linkname string
}

func TestInstallScriptHappyPath(t *testing.T) {
	skipWindows(t)
	fixture := newReleaseFixture(t, false)
	defer fixture.server.Close()

	binDir := t.TempDir()
	result := runInstall(t, fixture.server.URL, map[string]string{}, "--version", fixtureVersion, "--dir", binDir)
	assertExit(t, result, 0)
	assertInstalledBinary(t, filepath.Join(binDir, "chab"))
	if _, err := os.Stat(filepath.Join(binDir, "README.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("README.md was installed or stat failed: %v", err)
	}
}

func TestInstallScriptLatestResolution(t *testing.T) {
	skipWindows(t)
	fixture := newReleaseFixture(t, false)
	defer fixture.server.Close()

	binDir := t.TempDir()
	result := runInstall(t, fixture.server.URL, map[string]string{}, "--dir", binDir)
	assertExit(t, result, 0)
	assertInstalledBinary(t, filepath.Join(binDir, "chab"))
	if fixture.requestCount() < 3 {
		t.Fatalf("request count = %d, want latest plus downloads", fixture.requestCount())
	}
}

func TestInstallScriptLatestResolutionWithWgetAnnotatedLocation(t *testing.T) {
	skipWindows(t)
	fixture := newReleaseFixture(t, false)
	defer fixture.server.Close()

	// Force the installer onto its wget branch even on developer and CI hosts
	// where curl is normally present.
	toolPath, curlPath := wgetOnlyToolPath(t)
	binDir := t.TempDir()
	result := runInstall(t, fixture.server.URL, map[string]string{
		"PATH":                   toolPath,
		"CHAB_INSTALL_REAL_CURL": curlPath,
	}, "--dir", binDir)
	assertExit(t, result, 0)
	assertInstalledBinary(t, filepath.Join(binDir, "chab"))
}

func TestInstallScriptVersionNormalization(t *testing.T) {
	skipWindows(t)
	for _, version := range []string{"0.6.0", "v0.6.0"} {
		t.Run(version, func(t *testing.T) {
			fixture := newReleaseFixture(t, false)
			defer fixture.server.Close()

			binDir := t.TempDir()
			result := runInstall(t, fixture.server.URL, map[string]string{}, "--version", version, "--dir", binDir)
			assertExit(t, result, 0)
			assertInstalledBinary(t, filepath.Join(binDir, "chab"))
		})
	}
}

func TestInstallScriptInvalidVersionMakesNoRequests(t *testing.T) {
	skipWindows(t)
	tests := [][]string{
		{"--version", "latest-ish"},
		{"--version", "v0.6.0-rc1"},
		{"--version"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			fixture := newReleaseFixture(t, false)
			defer fixture.server.Close()

			binDir := t.TempDir()
			fullArgs := append(append([]string{}, args...), "--dir", binDir)
			result := runInstall(t, fixture.server.URL, map[string]string{}, fullArgs...)
			assertExit(t, result, 2)
			if !strings.Contains(result.stderr, "Usage: scripts/install.sh") {
				t.Fatalf("stderr missing usage:\n%s", result.stderr)
			}
			assertNoRequests(t, fixture)
			assertNoBinary(t, filepath.Join(binDir, "chab"))
		})
	}
}

func TestInstallScriptChecksumMismatchDoesNotInstall(t *testing.T) {
	skipWindows(t)
	fixture := newReleaseFixture(t, true)
	defer fixture.server.Close()

	binDir := t.TempDir()
	writeBinary(t, filepath.Join(binDir, "chab"), existingBinary, 0o755)
	result := runInstall(t, fixture.server.URL, map[string]string{}, "--version", fixtureVersion, "--dir", binDir)
	if result.exitCode == 0 {
		t.Fatalf("exit = 0, want failure; stdout=%s stderr=%s", result.stdout, result.stderr)
	}
	if !strings.Contains(result.stderr, "Checksum verification failed") {
		t.Fatalf("stderr missing checksum failure:\n%s", result.stderr)
	}
	assertBinaryContent(t, filepath.Join(binDir, "chab"), existingBinary)
}

func TestInstallScriptChecksumMismatchDoesNotInspectArchive(t *testing.T) {
	skipWindows(t)
	fixture := newReleaseFixture(t, true)
	defer fixture.server.Close()

	binDir := t.TempDir()
	result := runInstall(t, fixture.server.URL, map[string]string{
		"PATH": toolPathWithFailingCommand(t, "tar", "tar should not run"),
	}, "--version", fixtureVersion, "--dir", binDir)
	if result.exitCode == 0 {
		t.Fatalf("exit = 0, want failure; stdout=%s stderr=%s", result.stdout, result.stderr)
	}
	if !strings.Contains(result.stderr, "Checksum verification failed") {
		t.Fatalf("stderr missing checksum failure:\n%s", result.stderr)
	}
	if strings.Contains(result.stderr, "tar should not run") {
		t.Fatalf("tar was invoked before checksum rejection:\n%s", result.stderr)
	}
	assertNoBinary(t, filepath.Join(binDir, "chab"))
}

func TestInstallScriptMalformedArchivePreservesExistingBinary(t *testing.T) {
	skipWindows(t)
	fixture := newReleaseFixtureWithArchive(t, []byte("not a gzip tar archive\n"), false)
	defer fixture.server.Close()

	binDir := t.TempDir()
	writeBinary(t, filepath.Join(binDir, "chab"), existingBinary, 0o755)
	result := runInstall(t, fixture.server.URL, map[string]string{}, "--version", fixtureVersion, "--dir", binDir)
	if result.exitCode == 0 {
		t.Fatalf("exit = 0, want failure; stdout=%s stderr=%s", result.stdout, result.stderr)
	}
	if !strings.Contains(result.stderr, "Could not inspect archive member names") {
		t.Fatalf("stderr missing archive inspection failure:\n%s", result.stderr)
	}
	assertBinaryContent(t, filepath.Join(binDir, "chab"), existingBinary)
}

func TestInstallScriptRejectsUnsafeArchiveMembers(t *testing.T) {
	skipWindows(t)
	tests := []struct {
		name    string
		archive []byte
		wantErr string
	}{
		{
			name:    "absolute path",
			archive: buildArchiveWithEntries(t, tarEntry{name: "/chab", data: []byte(fixtureBinary), mode: 0o755}),
			wantErr: "unsafe member path",
		},
		{
			name:    "path traversal",
			archive: buildArchiveWithEntries(t, tarEntry{name: "../chab", data: []byte(fixtureBinary), mode: 0o755}),
			wantErr: "unsafe member path",
		},
		{
			name: "nested duplicate candidate",
			archive: buildArchiveWithEntries(t,
				tarEntry{name: "chab", data: []byte(fixtureBinary), mode: 0o755},
				tarEntry{name: "bin/chab", data: []byte("nested\n"), mode: 0o755},
			),
			wantErr: "exactly one regular top-level chab binary",
		},
		{
			name: "duplicate top-level candidate",
			archive: buildArchiveWithEntries(t,
				tarEntry{name: "chab", data: []byte(fixtureBinary), mode: 0o755},
				tarEntry{name: "chab", data: []byte("second\n"), mode: 0o755},
			),
			wantErr: "exactly one regular top-level chab binary",
		},
		{
			name: "symlink",
			archive: buildArchiveWithEntries(t,
				tarEntry{name: "README.md", data: []byte("companion\n"), mode: 0o644},
				tarEntry{name: "chab", mode: 0o777, typeflag: tar.TypeSymlink, linkname: "README.md"},
			),
			wantErr: "symlink member",
		},
		{
			name: "hardlink",
			archive: buildArchiveWithEntries(t,
				tarEntry{name: "README.md", data: []byte("companion\n"), mode: 0o644},
				tarEntry{name: "chab", mode: 0o777, typeflag: tar.TypeLink, linkname: "README.md"},
			),
			wantErr: "hardlink member",
		},
		{
			name:    "directory",
			archive: buildArchiveWithEntries(t, tarEntry{name: "chab", mode: 0o755, typeflag: tar.TypeDir}),
			wantErr: "non-regular member",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newReleaseFixtureWithArchive(t, tc.archive, false)
			defer fixture.server.Close()

			binDir := t.TempDir()
			writeBinary(t, filepath.Join(binDir, "chab"), existingBinary, 0o755)
			result := runInstall(t, fixture.server.URL, map[string]string{}, "--version", fixtureVersion, "--dir", binDir)
			if result.exitCode == 0 {
				t.Fatalf("exit = 0, want failure; stdout=%s stderr=%s", result.stdout, result.stderr)
			}
			if !strings.Contains(result.stderr, tc.wantErr) {
				t.Fatalf("stderr missing %q:\n%s", tc.wantErr, result.stderr)
			}
			assertBinaryContent(t, filepath.Join(binDir, "chab"), existingBinary)
			assertNoInstallTemps(t, binDir)
		})
	}
}

func TestInstallScriptFailedAtomicReplacementPreservesExistingBinary(t *testing.T) {
	skipWindows(t)
	fixture := newReleaseFixture(t, false)
	defer fixture.server.Close()

	binDir := t.TempDir()
	writeBinary(t, filepath.Join(binDir, "chab"), existingBinary, 0o755)
	result := runInstall(t, fixture.server.URL, map[string]string{
		"PATH": toolPathWithFailingCommand(t, "mv", "mv disabled by test"),
	}, "--version", fixtureVersion, "--dir", binDir)
	if result.exitCode == 0 {
		t.Fatalf("exit = 0, want failure; stdout=%s stderr=%s", result.stdout, result.stderr)
	}
	if !strings.Contains(result.stderr, "Could not install chab atomically") {
		t.Fatalf("stderr missing atomic install failure:\n%s", result.stderr)
	}
	assertBinaryContent(t, filepath.Join(binDir, "chab"), existingBinary)
	assertNoInstallTemps(t, binDir)
}

func TestInstallScriptNonRegularTargetPreservesExistingPath(t *testing.T) {
	skipWindows(t)
	fixture := newReleaseFixture(t, false)
	defer fixture.server.Close()

	binDir := t.TempDir()
	target := filepath.Join(binDir, "chab")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	result := runInstall(t, fixture.server.URL, map[string]string{}, "--version", fixtureVersion, "--dir", binDir)
	if result.exitCode == 0 {
		t.Fatalf("exit = 0, want failure; stdout=%s stderr=%s", result.stdout, result.stderr)
	}
	if !strings.Contains(result.stderr, "Install target exists and is not a regular file") {
		t.Fatalf("stderr missing non-regular target failure:\n%s", result.stderr)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("target mode = %s, want preserved directory", info.Mode())
	}
	assertNoInstallTemps(t, binDir)
}

func TestInstallScriptUnwritableDestinationDoesNotInstall(t *testing.T) {
	skipWindows(t)
	if os.Geteuid() == 0 {
		t.Skip("root can write through directory permissions")
	}
	fixture := newReleaseFixture(t, false)
	defer fixture.server.Close()

	parent := filepath.Join(t.TempDir(), "parent")
	if err := os.Mkdir(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(parent, 0o755)
	})
	target := filepath.Join(parent, "bin")
	result := runInstall(t, fixture.server.URL, map[string]string{}, "--version", fixtureVersion, "--dir", target)
	if result.exitCode == 0 {
		t.Fatalf("exit = 0, want failure; stdout=%s stderr=%s", result.stdout, result.stderr)
	}
	assertNoBinary(t, filepath.Join(target, "chab"))
}

func TestInstallScriptWindowsBranchMakesNoRequests(t *testing.T) {
	skipWindows(t)
	fixture := newReleaseFixture(t, false)
	defer fixture.server.Close()

	binDir := t.TempDir()
	result := runInstall(t, fixture.server.URL, map[string]string{"CHAB_INSTALL_OS": "windows"}, "--version", fixtureVersion, "--dir", binDir)
	if result.exitCode == 0 {
		t.Fatalf("exit = 0, want failure")
	}
	if !strings.Contains(result.stderr, "docs/install.md") {
		t.Fatalf("stderr missing manual docs pointer:\n%s", result.stderr)
	}
	assertNoRequests(t, fixture)
	assertNoBinary(t, filepath.Join(binDir, "chab"))
}

func TestInstallScriptUnsupportedPlatformMakesNoRequests(t *testing.T) {
	skipWindows(t)
	fixture := newReleaseFixture(t, false)
	defer fixture.server.Close()

	binDir := t.TempDir()
	result := runInstall(t, fixture.server.URL, map[string]string{"CHAB_INSTALL_OS": "plan9"}, "--version", fixtureVersion, "--dir", binDir)
	if result.exitCode == 0 {
		t.Fatalf("exit = 0, want failure")
	}
	if !strings.Contains(result.stderr, "Unsupported operating system") {
		t.Fatalf("stderr missing unsupported OS message:\n%s", result.stderr)
	}
	assertNoRequests(t, fixture)
	assertNoBinary(t, filepath.Join(binDir, "chab"))
}

func TestInstallScriptMissingToolsMakeNoRequests(t *testing.T) {
	skipWindows(t)
	for _, disabled := range []string{"downloader", "tar", "checksum"} {
		t.Run(disabled, func(t *testing.T) {
			fixture := newReleaseFixture(t, false)
			defer fixture.server.Close()

			binDir := t.TempDir()
			result := runInstall(t, fixture.server.URL, map[string]string{
				"CHAB_INSTALL_DISABLE_TOOLS": disabled,
			}, "--version", fixtureVersion, "--dir", binDir)
			if result.exitCode == 0 {
				t.Fatalf("exit = 0, want failure")
			}
			if !strings.Contains(strings.ToLower(result.stderr), disabled) {
				t.Fatalf("stderr = %q, want mention %q", result.stderr, disabled)
			}
			assertNoRequests(t, fixture)
			assertNoBinary(t, filepath.Join(binDir, "chab"))
		})
	}
}

func TestInstallScriptHelpMakesNoRequests(t *testing.T) {
	skipWindows(t)
	fixture := newReleaseFixture(t, false)
	defer fixture.server.Close()

	result := runInstall(t, fixture.server.URL, map[string]string{}, "--help")
	assertExit(t, result, 0)
	if !strings.Contains(result.stdout, "Usage: scripts/install.sh") {
		t.Fatalf("stdout missing usage:\n%s", result.stdout)
	}
	assertNoRequests(t, fixture)
}

func newReleaseFixture(t *testing.T, wrongChecksum bool) *releaseFixture {
	t.Helper()
	return newReleaseFixtureWithArchive(t, buildArchive(t), wrongChecksum)
}

func newReleaseFixtureWithArchive(t *testing.T, archive []byte, wrongChecksum bool) *releaseFixture {
	t.Helper()
	sum := fmt.Sprintf("%x", sha256.Sum256(archive))
	if wrongChecksum {
		sum = strings.Repeat("0", 64)
	}
	archiveName := "chab_" + fixtureVersion + "_linux_amd64.tar.gz"
	checksumsName := "chab_" + fixtureVersion + "_checksums.txt"

	fixture := &releaseFixture{}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.requests.Add(1)
		switch r.URL.Path {
		case "/" + "vincentsch/chab-cli/releases/latest":
			http.Redirect(w, r, "http://"+r.Host+"/vincentsch/chab-cli/releases/tag/v"+fixtureVersion, http.StatusFound)
		case "/" + "vincentsch/chab-cli/releases/tag/v" + fixtureVersion:
			_, _ = w.Write([]byte("release page"))
		case "/" + "vincentsch/chab-cli/releases/download/v" + fixtureVersion + "/" + archiveName:
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(archive)
		case "/" + "vincentsch/chab-cli/releases/download/v" + fixtureVersion + "/" + checksumsName:
			_, _ = fmt.Fprintf(w, "%s  %s\n", sum, archiveName)
		default:
			http.NotFound(w, r)
		}
	}))
	return fixture
}

func buildArchive(t *testing.T) []byte {
	t.Helper()
	return buildArchiveWithEntries(t,
		tarEntry{name: "chab", data: []byte(fixtureBinary), mode: 0o755},
		tarEntry{name: "README.md", data: []byte("companion\n"), mode: 0o644},
	)
}

func buildArchiveWithEntries(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		writeTarEntry(t, tw, entry)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeTarEntry(t *testing.T, tw *tar.Writer, entry tarEntry) {
	t.Helper()
	typeflag := entry.typeflag
	if typeflag == 0 {
		typeflag = tar.TypeReg
	}
	header := &tar.Header{
		Name:     entry.name,
		Mode:     entry.mode,
		Typeflag: typeflag,
		Linkname: entry.linkname,
	}
	if typeflag == tar.TypeReg || typeflag == tar.TypeRegA {
		header.Size = int64(len(entry.data))
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if header.Size == 0 {
		return
	}
	if _, err := tw.Write(entry.data); err != nil {
		t.Fatal(err)
	}
}

func runInstall(t *testing.T, baseURL string, extraEnv map[string]string, args ...string) scriptResult {
	t.Helper()
	repoRoot := findRepoRoot(t)
	cmd := exec.Command("sh", append([]string{filepath.Join(repoRoot, "scripts/install.sh")}, args...)...)
	cmd.Env = installEnv(baseURL, extraEnv)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("run install script: %v", err)
		}
	}
	return scriptResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}
}

func installEnv(baseURL string, extraEnv map[string]string) []string {
	// Preserve the host environment except for installer test seams and keys
	// that a test explicitly overrides, such as PATH in the wget-only case.
	overrides := make(map[string]bool, len(extraEnv))
	for key := range extraEnv {
		overrides[key] = true
	}
	env := make([]string, 0, len(os.Environ())+3+len(extraEnv))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "CHAB_INSTALL_") || overrides[key] {
			continue
		}
		env = append(env, entry)
	}
	values := map[string]string{
		"CHAB_INSTALL_BASE_URL": baseURL,
		"CHAB_INSTALL_OS":       "linux",
		"CHAB_INSTALL_ARCH":     "amd64",
	}
	for key, value := range extraEnv {
		values[key] = value
	}
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}

func wgetOnlyToolPath(t *testing.T) (string, string) {
	t.Helper()
	// The fake wget delegates downloads to real curl by absolute path. curl is
	// intentionally absent from PATH so the installer still selects wget.
	curlPath, err := exec.LookPath("curl")
	if err != nil {
		t.Skip("curl is required to back the fake wget downloader")
	}

	dir := t.TempDir()
	for _, name := range []string{"chmod", "cp", "gzip", "mkdir", "mktemp", "mv", "rm", "sed", "tar"} {
		linkRequiredTool(t, dir, name)
	}
	if shaPath, err := exec.LookPath("sha256sum"); err == nil {
		linkToolPath(t, dir, "sha256sum", shaPath)
	} else {
		linkRequiredTool(t, dir, "shasum")
	}

	// GNU wget prints both the redirect target and the followed target with a
	// trailing " [following]" annotation. The installer deliberately reads the
	// last Location line, so this fixture keeps the risky shape visible.
	wgetScript := `#!/bin/sh
set -eu

if [ "$#" -eq 3 ] && [ "$1" = "-S" ] && [ "$2" = "--spider" ]; then
	printf '  Location: %s/vincentsch/chab-cli/releases/tag/v0.6.0\n' "$CHAB_INSTALL_BASE_URL" >&2
	printf '  Location: %s/vincentsch/chab-cli/releases/tag/v0.6.0 [following]\n' "$CHAB_INSTALL_BASE_URL" >&2
	exit 0
fi

if [ "$#" -eq 4 ] && [ "$1" = "-q" ] && [ "$2" = "-O" ]; then
	exec "$CHAB_INSTALL_REAL_CURL" -fsSL -o "$3" "$4"
fi

printf 'unexpected wget args:' >&2
for arg in "$@"; do
	printf ' %s' "$arg" >&2
done
printf '\n' >&2
exit 2
`
	if err := os.WriteFile(filepath.Join(dir, "wget"), []byte(wgetScript), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, curlPath
}

func toolPathWithFailingCommand(t *testing.T, failingName, message string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"chmod", "cp", "curl", "gzip", "mkdir", "mktemp", "rm", "sed", "tar"} {
		if name == failingName {
			continue
		}
		linkRequiredTool(t, dir, name)
	}
	if shaPath, err := exec.LookPath("sha256sum"); err == nil {
		linkToolPath(t, dir, "sha256sum", shaPath)
	} else {
		linkRequiredTool(t, dir, "shasum")
	}
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' %q >&2\nexit 1\n", message)
	if err := os.WriteFile(filepath.Join(dir, failingName), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func linkRequiredTool(t *testing.T, dir, name string) {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is required for install script path-isolation test", name)
	}
	linkToolPath(t, dir, name, path)
}

func linkToolPath(t *testing.T, dir, name, path string) {
	t.Helper()
	if err := os.Symlink(path, filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
}

func assertExit(t *testing.T, result scriptResult, want int) {
	t.Helper()
	if result.exitCode != want {
		t.Fatalf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", result.exitCode, want, result.stdout, result.stderr)
	}
}

func assertInstalledBinary(t *testing.T, path string) {
	t.Helper()
	assertBinaryContent(t, path, fixtureBinary)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("%s is not executable: mode %s", path, info.Mode())
	}
}

func assertBinaryContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("binary content at %s = %q, want %q", path, data, want)
	}
}

func assertNoBinary(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("binary exists or stat failed for %s: %v", path, err)
	}
}

func assertNoInstallTemps(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".chab.tmp.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary install files remained: %v", matches)
	}
}

func writeBinary(t *testing.T, path, data string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}

func assertNoRequests(t *testing.T, fixture *releaseFixture) {
	t.Helper()
	if got := fixture.requestCount(); got != 0 {
		t.Fatalf("request count = %d, want 0", got)
	}
}

func (f *releaseFixture) requestCount() int64 {
	return f.requests.Load()
}

func skipWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install script tests use POSIX sh")
	}
}

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
