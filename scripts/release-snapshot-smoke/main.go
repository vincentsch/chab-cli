// Command release-snapshot-smoke builds a GoReleaser snapshot and verifies that
// the produced archives, checksum file, safe binary members, installer path,
// installed no-secret smoke, and injected build metadata match the release
// contract in docs/release-playbook.md.
//
// It shells out to goreleaser (which must be on PATH, pinned to v2.16.0 in CI
// and locally). The executable smoke path is deliberately kept out of the
// default Go test suite: package tests cover pure helpers only and must not call
// run(), so GoReleaser never becomes a dependency of `go test ./...`. Invoke the
// full check with `go run ./scripts/release-snapshot-smoke`.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
)

const distDir = "dist"

// archiveMeta is the release-contract view of one artifact. It stores only the
// facts the smoke proves from the archive itself.
type archiveMeta struct {
	format string // "tar.gz" or "zip"
	binary string // "chab", "chab.exe", or empty for the standalone skill
	goos   string
	goarch string
}

type archiveEntry struct {
	name    string
	regular bool
	link    bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "snapshot smoke failed:", err)
		os.Exit(1)
	}
	fmt.Println("snapshot smoke passed")
}

func run() error {
	if _, err := exec.LookPath("goreleaser"); err != nil {
		return fmt.Errorf("goreleaser not on PATH (install GoReleaser v2.16.0): %w", err)
	}

	// Build the snapshot without publishing. --clean wipes dist first.
	build := exec.Command("goreleaser", "release", "--snapshot", "--clean")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("goreleaser snapshot build: %w", err)
	}

	archives, err := globArchives()
	if err != nil {
		return err
	}

	version, err := deriveVersion(archives)
	if err != nil {
		return err
	}
	expected := expectedArchives(version)

	// Set equality catches both missing targets and accidental extras such as
	// windows/arm64. These file names are the public install contract.
	if err := assertSameSet("release archives", basenames(archives), keys(expected)); err != nil {
		return err
	}
	if err := verifyChecksums(version, expected); err != nil {
		return err
	}

	if err := verifyArchiveContents(expected); err != nil {
		return err
	}
	if err := verifyCurrentPlatform(expected); err != nil {
		return err
	}
	return verifyInstallerFlow(version, expected)
}

// expectedArchives returns the complete release artifact contract for one
// normalized version string.
func expectedArchives(v string) map[string]archiveMeta {
	return map[string]archiveMeta{
		fmt.Sprintf("chab_%s_agent.zip", v):           {"zip", "", "", ""},
		fmt.Sprintf("chab_%s_linux_amd64.tar.gz", v):  {"tar.gz", "chab", "linux", "amd64"},
		fmt.Sprintf("chab_%s_linux_arm64.tar.gz", v):  {"tar.gz", "chab", "linux", "arm64"},
		fmt.Sprintf("chab_%s_darwin_amd64.tar.gz", v): {"tar.gz", "chab", "darwin", "amd64"},
		fmt.Sprintf("chab_%s_darwin_arm64.tar.gz", v): {"tar.gz", "chab", "darwin", "arm64"},
		fmt.Sprintf("chab_%s_windows_amd64.zip", v):   {"zip", "chab.exe", "windows", "amd64"},
	}
}

// globArchives discovers produced release archives without reading
// dist/artifacts.json, keeping the smoke independent of GoReleaser metadata
// schema details.
func globArchives() ([]string, error) {
	var out []string
	for _, pattern := range []string{"*.tar.gz", "*.zip"} {
		matches, err := filepath.Glob(filepath.Join(distDir, pattern))
		if err != nil {
			return nil, err
		}
		out = append(out, matches...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no archives produced in %s/", distDir)
	}
	return out, nil
}

var versionRE = regexp.MustCompile(`^chab_(.+)_linux_amd64\.tar\.gz$`)

// deriveVersion uses the required linux/amd64 archive as the version source.
// The full archive set check immediately after this call proves the other
// target names use the same version.
func deriveVersion(archives []string) (string, error) {
	for _, archive := range archives {
		if match := versionRE.FindStringSubmatch(filepath.Base(archive)); match != nil {
			return match[1], nil
		}
	}
	return "", fmt.Errorf("could not derive version from archives: %v", basenames(archives))
}

// verifyChecksums proves both checksum-file coverage and checksum correctness.
// It compares basenames because that is what release installers consume.
func verifyChecksums(version string, expected map[string]archiveMeta) error {
	name := fmt.Sprintf("chab_%s_checksums.txt", version)
	data, err := os.ReadFile(filepath.Join(distDir, name))
	if err != nil {
		return fmt.Errorf("checksum file %s: %w", name, err)
	}

	listed := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return fmt.Errorf("unexpected checksum line %q", line)
		}
		listed[fields[1]] = fields[0]
	}
	if err := assertSameSet("checksum coverage", keysOf(listed), keys(expected)); err != nil {
		return err
	}

	for name, want := range listed {
		got, err := sha256File(filepath.Join(distDir, name))
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("checksum mismatch for %s: file=%s listed=%s", name, got, want)
		}
	}
	return nil
}

// verifyArchiveContents lists and manually extracts every target archive. That
// gives Windows zip archives the same checksum and extraction coverage as the
// tar.gz archives while only executing the current platform binary later.
func verifyArchiveContents(expected map[string]archiveMeta) error {
	for name, meta := range expected {
		archivePath := filepath.Join(distDir, name)
		if meta.binary == "" {
			skill, err := os.ReadFile(".agents/skills/chab/SKILL.md")
			if err != nil {
				return err
			}
			if err := verifyAgentArchive(archivePath, skill); err != nil {
				return err
			}
			continue
		}
		entries, err := listArchive(archivePath, meta.format)
		if err != nil {
			return err
		}
		if err := assertArchiveEntries(name, entries, meta.binary); err != nil {
			return err
		}

		dir, err := os.MkdirTemp("", "chab-manual-extract")
		if err != nil {
			return err
		}
		binPath, extractErr := extractBinary(archivePath, meta, dir)
		if extractErr == nil {
			var info os.FileInfo
			info, extractErr = os.Stat(binPath)
			if extractErr == nil && info.Size() == 0 {
				extractErr = fmt.Errorf("extracted binary is empty")
			}
		}
		removeErr := os.RemoveAll(dir)
		if extractErr != nil {
			return fmt.Errorf("manual extraction for %s: %w", name, extractErr)
		}
		if removeErr != nil {
			return removeErr
		}
	}
	return nil
}

func assertArchiveEntries(archiveName string, entries []archiveEntry, binary string) error {
	topLevelRegular := 0
	binaryCandidates := 0
	for _, entry := range entries {
		if !archiveNameIsSafe(entry.name) {
			return fmt.Errorf("archive %s contains unsafe member path %q", archiveName, entry.name)
		}
		if entry.link {
			return fmt.Errorf("archive %s contains link member %q", archiveName, entry.name)
		}
		if !entry.regular {
			return fmt.Errorf("archive %s contains non-regular member %q", archiveName, entry.name)
		}
		trimmed := strings.TrimRight(entry.name, "/")
		if path.Base(trimmed) == binary {
			binaryCandidates++
		}
		if entry.name == binary && entry.regular {
			topLevelRegular++
		}
	}
	if topLevelRegular != 1 || binaryCandidates != 1 {
		return fmt.Errorf("archive %s must contain exactly one regular top-level %s binary (entries: %v)", archiveName, binary, entryNames(entries))
	}
	return nil
}

func archiveNameIsSafe(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

// verifyCurrentPlatform executes only the package built for the current
// machine. The other archives are still checked structurally above; executing
// cross-built binaries would make the smoke host-dependent.
func verifyCurrentPlatform(expected map[string]archiveMeta) error {
	var currentName string
	var currentMeta archiveMeta
	for name, meta := range expected {
		if meta.goos == runtime.GOOS && meta.goarch == runtime.GOARCH {
			currentName, currentMeta = name, meta
			break
		}
	}
	if currentName == "" {
		fmt.Printf("current platform %s/%s is not a release target; skipping executable smoke\n", runtime.GOOS, runtime.GOARCH)
		return nil
	}

	dir, err := os.MkdirTemp("", "chab-smoke")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	binPath, err := extractBinary(filepath.Join(distDir, currentName), currentMeta, dir)
	if err != nil {
		return err
	}

	got, err := verifyVersionJSON(binPath)
	if err != nil {
		return fmt.Errorf("run packaged binary: %w", err)
	}
	fmt.Printf("packaged %s reports version=%s commit=%s date=%s\n", currentName, got["version"], got["commit"], got["date"])
	return nil
}

func verifyInstallerFlow(version string, expected map[string]archiveMeta) error {
	var currentName string
	var currentMeta archiveMeta
	for name, meta := range expected {
		if meta.goos == runtime.GOOS && meta.goarch == runtime.GOARCH {
			currentName, currentMeta = name, meta
			break
		}
	}
	if currentName == "" {
		fmt.Printf("current platform %s/%s is not a release target; skipping installer smoke\n", runtime.GOOS, runtime.GOARCH)
		return nil
	}
	if currentMeta.goos == "windows" {
		fmt.Println("POSIX installer does not install Windows archives; manual extraction coverage already passed")
		return nil
	}

	server, archiveRequests, checksumRequests := startReleaseAssetServer(version, currentName, expected)
	defer server.Close()

	installDir, err := os.MkdirTemp("", "chab-install")
	if err != nil {
		return err
	}
	defer os.RemoveAll(installDir)

	cmd := exec.Command("sh", "scripts/install.sh", "--version", version, "--dir", installDir)
	cmd.Env = installerEnv(server.URL, currentMeta.goos, currentMeta.goarch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install snapshot archive through installer: %w\n%s", err, out)
	}
	if got := archiveRequests.Load(); got != 1 {
		return fmt.Errorf("installer downloaded %s %d times, want 1", currentName, got)
	}
	if got := checksumRequests.Load(); got != 1 {
		return fmt.Errorf("installer downloaded checksum file %d times, want 1", got)
	}

	installed := filepath.Join(installDir, "chab")
	info, err := os.Stat(installed)
	if err != nil {
		return fmt.Errorf("installed binary: %w", err)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("installed binary is not executable: %s", info.Mode())
	}
	got, err := verifyVersionJSON(installed)
	if err != nil {
		return fmt.Errorf("run installed binary: %w", err)
	}

	smoke := exec.Command("sh", "scripts/smoke-release-no-secret.sh", "--bin", installed)
	smokeOut, err := smoke.CombinedOutput()
	if err != nil {
		return fmt.Errorf("installed binary no-secret smoke: %w\n%s", err, smokeOut)
	}
	fmt.Printf("installer served %s and installed version=%s commit=%s date=%s\n", currentName, got["version"], got["commit"], got["date"])
	return nil
}

func startReleaseAssetServer(version, currentArchive string, expected map[string]archiveMeta) (*httptest.Server, *atomic.Int64, *atomic.Int64) {
	checksumsName := fmt.Sprintf("chab_%s_checksums.txt", version)
	allowed := map[string]bool{checksumsName: true}
	for name := range expected {
		allowed[name] = true
	}
	var archiveRequests atomic.Int64
	var checksumRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/vincentsch/chab-cli/releases/latest":
			http.Redirect(w, r, "/vincentsch/chab-cli/releases/tag/v"+version, http.StatusFound)
			return
		case "/vincentsch/chab-cli/releases/tag/v" + version:
			_, _ = w.Write([]byte("snapshot release"))
			return
		}

		prefix := "/vincentsch/chab-cli/releases/download/v" + version + "/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, prefix)
		if !allowed[name] || path.Base(name) != name {
			http.NotFound(w, r)
			return
		}
		if name == currentArchive {
			archiveRequests.Add(1)
		}
		if name == checksumsName {
			checksumRequests.Add(1)
		}
		http.ServeFile(w, r, filepath.Join(distDir, name))
	}))
	return server, &archiveRequests, &checksumRequests
}

func installerEnv(baseURL, osToken, archToken string) []string {
	env := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "CHAB_INSTALL_") {
			continue
		}
		env = append(env, entry)
	}
	return append(env,
		"CHAB_INSTALL_BASE_URL="+baseURL,
		"CHAB_INSTALL_OS="+osToken,
		"CHAB_INSTALL_ARCH="+archToken,
	)
}

func verifyVersionJSON(binPath string) (map[string]any, error) {
	out, err := exec.Command(binPath, "version", "--json").Output()
	if err != nil {
		return nil, err
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		return nil, fmt.Errorf("version --json not JSON: %w\n%s", err, out)
	}
	wantKeys := []string{"version", "commit", "date", "go_version", "os", "arch"}
	if len(got) != len(wantKeys) {
		return nil, fmt.Errorf("version JSON key count = %d, want %d: %v", len(got), len(wantKeys), got)
	}
	for _, key := range wantKeys {
		if _, ok := got[key]; !ok {
			return nil, fmt.Errorf("version JSON missing key %q: %v", key, got)
		}
	}
	// The smoke does not need to know the exact release date or commit ahead of
	// time. It only needs to prove ldflags replaced the local-build fallbacks.
	for key, def := range map[string]string{"version": "dev", "commit": "none", "date": "unknown"} {
		value, _ := got[key].(string)
		if value == "" || value == def {
			return nil, fmt.Errorf("%s = %q, want injected non-default value", key, value)
		}
	}
	return got, nil
}

// listArchive returns archive member names for the two formats this project
// publishes.
func listArchive(path, format string) ([]archiveEntry, error) {
	switch format {
	case "tar.gz":
		return listTarGz(path)
	case "zip":
		return listZip(path)
	default:
		return nil, fmt.Errorf("unknown archive format %q", format)
	}
}

func listTarGz(path string) ([]archiveEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var entries []archiveEntry
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, archiveEntry{
			name:    header.Name,
			regular: header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA,
			link:    header.Typeflag == tar.TypeLink || header.Typeflag == tar.TypeSymlink,
		})
	}
	return entries, nil
}

func listZip(path string) ([]archiveEntry, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	var entries []archiveEntry
	for _, f := range zr.File {
		mode := f.FileInfo().Mode()
		entries = append(entries, archiveEntry{
			name:    f.Name,
			regular: mode.IsRegular(),
			link:    mode&os.ModeSymlink != 0,
		})
	}
	return entries, nil
}

// extractBinary copies the named binary from one archive to destDir so the
// smoke runs the packaged executable exactly as users receive it.
func extractBinary(archivePath string, meta archiveMeta, destDir string) (string, error) {
	dest := filepath.Join(destDir, meta.binary)
	switch meta.format {
	case "tar.gz":
		f, err := os.Open(archivePath)
		if err != nil {
			return "", err
		}
		defer f.Close()

		gz, err := gzip.NewReader(f)
		if err != nil {
			return "", err
		}
		defer gz.Close()

		tr := tar.NewReader(gz)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}
			if header.Name == meta.binary && (header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA) {
				if err := writeExecutable(dest, tr); err != nil {
					return "", err
				}
				return dest, nil
			}
		}
	case "zip":
		zr, err := zip.OpenReader(archivePath)
		if err != nil {
			return "", err
		}
		defer zr.Close()

		for _, zf := range zr.File {
			if zf.Name == meta.binary && zf.FileInfo().Mode().IsRegular() {
				rc, err := zf.Open()
				if err != nil {
					return "", err
				}
				defer rc.Close()
				if err := writeExecutable(dest, rc); err != nil {
					return "", err
				}
				return dest, nil
			}
		}
	}
	return "", fmt.Errorf("binary %q not found in %s", meta.binary, archivePath)
}

// writeExecutable writes an extracted archive member with executable bits set.
// Zip archives do not reliably preserve Unix execute mode across platforms.
func writeExecutable(dest string, r io.Reader) error {
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// sha256File returns the lowercase hex digest format used in GoReleaser's
// checksum file.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func basenames(paths []string) []string {
	out := make([]string, len(paths))
	for i, path := range paths {
		out[i] = filepath.Base(path)
	}
	return out
}

func keys(m map[string]archiveMeta) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}

func entryNames(entries []archiveEntry) []string {
	out := make([]string, len(entries))
	for i, entry := range entries {
		out[i] = entry.name
	}
	return out
}

// assertSameSet compares unordered file-name sets and reports sorted values so
// CI failures are readable.
func assertSameSet(label string, got, want []string) error {
	g := append([]string(nil), got...)
	w := append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	if strings.Join(g, "\n") != strings.Join(w, "\n") {
		return fmt.Errorf("%s mismatch:\n got: %v\nwant: %v", label, g, w)
	}
	return nil
}
