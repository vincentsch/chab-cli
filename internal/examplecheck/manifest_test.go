package examplecheck

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifestStrictDecodeAndTrailingData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"examples":[],"unexpected":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown-field error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"examples":[]} {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil || !strings.Contains(err.Error(), "trailing JSON data") {
		t.Fatalf("trailing-data error = %v", err)
	}
}

func TestValidateStructureReportsEveryRule(t *testing.T) {
	negative := -1
	example := Example{
		Script:         "examples/ci/bad.sh",
		Description:    " ",
		StdoutContains: []string{" "},
		StderrContains: []string{""},
		MockRequests:   &negative,
		Commands: []CommandUse{
			{Path: []string{"api", ""}, Flags: []string{"json", "json", ""}},
			{Path: []string{"api", ""}},
		},
	}
	got := joinErrors(ValidateStructure(Manifest{Examples: []Example{example}}))
	for _, want := range []string{
		"description must not be blank",
		"path[1] must not be blank",
		`duplicate flag "json"`,
		"flags must be strictly sorted",
		"flags[2] must not be blank",
		"stdout_contains[0] must not be blank",
		"stderr_contains[0] must not be blank",
		"at least one stdout or stderr assertion is required",
		"mock_requests must be nonnegative",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("errors missing %q:\n%s", want, got)
		}
	}

	missing := validExample("examples/ci/missing.sh")
	missing.Commands = nil
	missing.MockRequests = nil
	got = joinErrors(ValidateStructure(Manifest{Examples: []Example{missing}}))
	for _, want := range []string{"commands must not be empty", "mock_requests is required"} {
		if !strings.Contains(got, want) {
			t.Fatalf("errors missing %q:\n%s", want, got)
		}
	}
}

func TestValidateStructureDuplicatePathsAndSortedFlags(t *testing.T) {
	example := validExample("examples/ci/duplicate.sh")
	example.Commands = []CommandUse{
		{Path: []string{"api", "get"}, Flags: []string{"json", "include-meta"}},
		{Path: []string{"api", "get"}},
	}
	got := joinErrors(ValidateStructure(Manifest{Examples: []Example{example}}))
	for _, want := range []string{"flags must be strictly sorted", `duplicate command path "api get"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("errors missing %q:\n%s", want, got)
		}
	}
}

func TestValidateStructureRejectsWhitespaceInsidePathElements(t *testing.T) {
	example := validExample("examples/ci/combined-path.sh")
	example.Commands = []CommandUse{
		{Path: []string{"api", "get"}},
		{Path: []string{"api get"}},
	}
	got := joinErrors(ValidateStructure(Manifest{Examples: []Example{example}}))
	if !strings.Contains(got, "commands[1].path[0] must not contain whitespace") {
		t.Fatalf("whitespace-path errors:\n%s", got)
	}
	if strings.Contains(got, "duplicate command path") {
		t.Fatalf("structurally distinct paths were treated as duplicates:\n%s", got)
	}
}

func TestValidateLayoutAndDiscoveryBoundaries(t *testing.T) {
	discovered := []string{"examples/ci/list.sh", "examples/support/raw.sh"}
	manifest := Manifest{Examples: []Example{
		{Script: "examples/ci/list.sh"},
		{Script: "examples/ci/missing.sh"},
		{Script: "examples/live/whoami.sh"},
		{Script: "examples/ci/list.sh"},
		{Script: "../outside.sh"},
		{Script: ""},
	}}
	got := joinErrors(ValidateLayout(manifest, discovered))
	for _, want := range []string{
		"examples/ci/missing.sh: manifest entry does not exist",
		"examples/live/whoami.sh: script must be under",
		"examples/ci/list.sh: duplicate manifest entry",
		"../outside.sh: script must be under",
		"manifest entry has empty script",
		"examples/support/raw.sh: discovered script missing from manifest",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("errors missing %q:\n%s", want, got)
		}
	}

	root := t.TempDir()
	for _, rel := range []string{
		"examples/ci/list.sh",
		"examples/support/show.sh",
		"examples/agents/write.sh",
		"examples/live/ignored.sh",
	} {
		writeTestFile(t, root, rel, []byte("#!/bin/sh\n"), 0o755)
	}
	scripts, err := DiscoverScripts(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"examples/agents/write.sh", "examples/ci/list.sh", "examples/support/show.sh"}
	if strings.Join(scripts, "\n") != strings.Join(want, "\n") {
		t.Fatalf("scripts = %#v, want %#v", scripts, want)
	}
}

func TestValidateScriptModesAndLineCountBoundary(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "examples/ci/not-exec.sh", []byte("#!/bin/sh\n"), 0o644)
	if got := joinErrors(ValidateScriptModes(root, []string{"examples/ci/not-exec.sh"})); !strings.Contains(got, "not executable") {
		t.Fatalf("mode errors = %q", got)
	}

	eighty := append(bytes.Repeat([]byte("x\n"), 79), 'x')
	writeTestFile(t, root, "examples/ci/eighty.sh", eighty, 0o755)
	if errs := ValidateScriptLineCounts(root, []string{"examples/ci/eighty.sh"}); len(errs) != 0 {
		t.Fatalf("80-line errors = %#v", errs)
	}
	eightyOne := append(append([]byte(nil), eighty...), []byte("\ny")...)
	writeTestFile(t, root, "examples/ci/eighty-one.sh", eightyOne, 0o755)
	got := joinErrors(ValidateScriptLineCounts(root, []string{"examples/ci/eighty-one.sh"}))
	if !strings.Contains(got, "81 lines, maximum is 80") {
		t.Fatalf("line-count errors = %q", got)
	}
}

func TestValidateSafetyIncludesManifestAndUnlistedScript(t *testing.T) {
	root := t.TempDir()
	token := strings.Join([]string{"sample", "credential"}, "|")
	writeTestFile(t, root, manifestPath, []byte(`{"examples":[]}`), 0o644)
	writeTestFile(t, root, "examples/ci/unlisted.sh", []byte("#!/bin/sh\n# "+token+"\n"), 0o755)
	got := joinErrors(ValidateSafety(root, manifestPath, []string{"examples/ci/unlisted.sh"}))
	if !strings.Contains(got, "examples/ci/unlisted.sh: leaks key-shaped token") {
		t.Fatalf("safety errors = %q", got)
	}
}

func validExample(script string) Example {
	requests := 0
	return Example{
		Script:         script,
		Description:    "valid example",
		StdoutContains: []string{"ok"},
		MockRequests:   &requests,
		Commands:       []CommandUse{{Path: []string{"api", "get"}, Flags: []string{"json"}}},
	}
}

func writeTestFile(t *testing.T, root, rel string, data []byte, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func joinErrors(errs []error) string {
	var builder strings.Builder
	for _, err := range errs {
		builder.WriteString(err.Error())
		builder.WriteByte('\n')
	}
	return builder.String()
}
