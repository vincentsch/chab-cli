// Package isolationcheck contains file-backed tests that keep local planning
// artifacts out of runtime Go source.
package isolationcheck

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const planningArtifactNeedle = ".vroni"

// TestRuntimeSourceDoesNotReferencePlanningArtifacts fails if runtime Go files
// start depending on local planning artifacts. The package is run with
// -count=1 in CI because it reads files outside its own package directory.
func TestRuntimeSourceDoesNotReferencePlanningArtifacts(t *testing.T) {
	repoRoot := findRepoRoot(t)
	var violations []string

	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".vroni":
				// These directories are repository metadata, not runtime source.
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Contains(data, []byte(planningArtifactNeedle)) {
			return nil
		}

		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for lineNo, line := range strings.Split(string(data), "\n") {
			// Embed directives get a clearer message than ordinary references.
			if strings.Contains(line, "//go:embed") && strings.Contains(line, planningArtifactNeedle) {
				violations = append(violations, fmt.Sprintf("%s:%d embeds %s", rel, lineNo+1, planningArtifactNeedle))
				continue
			}
			if strings.Contains(line, planningArtifactNeedle) {
				violations = append(violations, fmt.Sprintf("%s:%d references %s", rel, lineNo+1, planningArtifactNeedle))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("runtime/planning isolation violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestRuntimeImportBoundaries(t *testing.T) {
	repoRoot := findRepoRoot(t)
	modulePath := "github.com/vincentsch/chab-cli"

	// Config and auth are foundational packages. Keep direct imports checked so
	// they cannot grow command-layer dependencies as retained code evolves.
	imports := goListLines(t, repoRoot, "-f", "{{range .Imports}}{{.}}{{\"\\n\"}}{{end}}", "./internal/config", "./internal/auth")
	for _, imp := range imports {
		switch {
		case imp == "github.com/spf13/cobra":
			t.Fatalf("config/auth must not import Cobra; found %s", imp)
		case imp == modulePath+"/internal/cli":
			t.Fatalf("config/auth must not import internal/cli")
		case imp == modulePath+"/internal/cmdutil":
			t.Fatalf("config/auth must not import command runtime adapters")
		case strings.HasPrefix(imp, modulePath+"/internal/commands"):
			t.Fatalf("config/auth must not import command packages; found %s", imp)
		}
	}

	// The reusable API runtime and command adapter layer must stay below the
	// active Cobra shell. Check production transitive dependencies so tests in
	// those packages can still use higher-level harnesses without weakening the
	// runtime boundary.
	runtimeDeps := goListLines(t, repoRoot, "-deps", "./internal/api", "./internal/cmdutil")
	for _, dep := range runtimeDeps {
		if dep == modulePath+"/internal/cli" {
			t.Fatalf("internal/api or internal/cmdutil depends on internal/cli")
		}
	}

	// The process entry point is intentionally thinner than many retained
	// packages in this repository, so check the full transitive dependency set.
	deps := goListLines(t, repoRoot, "-deps", "./cmd/chab")
	for _, dep := range deps {
		switch {
		case dep == modulePath+"/internal/update",
			dep == modulePath+"/internal/retainedcmdutil",
			dep == modulePath+"/internal/feature",
			dep == modulePath+"/internal/commandmeta":
			t.Fatalf("cmd/chab depends on later runtime package %s", dep)
		case dep == modulePath+"/internal/commands/authcmd",
			dep == modulePath+"/internal/commands/credits",
			dep == modulePath+"/internal/commands/doctor",
			dep == modulePath+"/internal/commands/management",
			dep == modulePath+"/internal/commands/mcpcmd",
			dep == modulePath+"/internal/commands/operations",
			dep == modulePath+"/internal/commands/profile",
			dep == modulePath+"/internal/commands/projects",
			dep == modulePath+"/internal/commands/rawapi",
			dep == modulePath+"/internal/commands/system":
			// These command packages are part of the active shell.
		case strings.HasPrefix(dep, modulePath+"/internal/commands/"):
			t.Fatalf("cmd/chab depends on retained command package %s", dep)
		case dep == modulePath+"/internal/examples/customer":
			t.Fatalf("cmd/chab depends on retained example package %s", dep)
		case strings.HasPrefix(dep, "github.com/vincentsch/rungrad"):
			t.Fatalf("cmd/chab depends on rungrad package %s", dep)
		}
	}
}

// findRepoRoot lets this test-only package run from any package working
// directory selected by `go test ./...`.
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

func goListLines(t *testing.T, repoRoot string, args ...string) []string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"list"}, args...)...)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list %v failed: %v\n%s", args, err, out)
	}
	fields := strings.Fields(string(out))
	return fields
}
