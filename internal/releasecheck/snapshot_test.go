package releasecheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoReleaserSnapshotVersionSupportsInstallerSmoke(t *testing.T) {
	repoRoot := findRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(repoRoot, ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"snapshot:",
		`version_template: "0.0.0"`,
		`name_template: "chab_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`,
		`name_template: "chab_{{ .Version }}_checksums.txt"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf(".goreleaser.yaml missing %q", want)
		}
	}
}

func TestReleaseWorkflowRunsSnapshotInstallerSmokeBeforePublish(t *testing.T) {
	repoRoot := findRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(".github/workflows/release.yml")))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	smoke := "go run ./scripts/release-snapshot-smoke"
	publish := "goreleaser release --clean"
	smokeAt := strings.Index(text, smoke)
	publishAt := strings.Index(text, publish)
	if smokeAt < 0 {
		t.Fatalf("release workflow missing %q", smoke)
	}
	if publishAt < 0 {
		t.Fatalf("release workflow missing %q", publish)
	}
	if smokeAt > publishAt {
		t.Fatalf("release workflow runs snapshot smoke after publish")
	}
}
