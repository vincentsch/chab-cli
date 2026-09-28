// Package examplecheck validates and runs the executable automation examples.
//
// The manifest is the source of truth for which commands each shell script is
// expected to exercise. This package intentionally never parses shell.
package examplecheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/vincentsch/chab-cli/internal/safetylint"
)

// Manifest mirrors examples/check-examples.json.
type Manifest struct {
	Examples []Example `json:"examples"`
}

// Example describes one executable non-live script and its expected outcome.
type Example struct {
	Script         string       `json:"script"`
	Description    string       `json:"description"`
	Exit           int          `json:"exit"`
	StdoutContains []string     `json:"stdout_contains,omitempty"`
	StderrContains []string     `json:"stderr_contains,omitempty"`
	MockRequests   *int         `json:"mock_requests,omitempty"`
	Commands       []CommandUse `json:"commands"`
}

// CommandUse declares a command path and the flags used by the script.
type CommandUse struct {
	Path  []string `json:"path"`
	Flags []string `json:"flags,omitempty"`
}

var allowedScriptDirs = []string{
	"examples/ci/",
	"examples/support/",
	"examples/agents/",
}

const maxScriptLines = 80

// LoadManifest reads a strict JSON manifest.
func LoadManifest(path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	dec.DisallowUnknownFields()
	var manifest Manifest
	if err := dec.Decode(&manifest); err != nil {
		return Manifest{}, err
	}
	var trailing struct{}
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, fmt.Errorf("manifest has trailing JSON data")
		}
		return Manifest{}, fmt.Errorf("manifest has trailing JSON data: %w", err)
	}
	return manifest, nil
}

// DiscoverScripts returns every regular file under the non-live example dirs.
func DiscoverScripts(repoRoot string) ([]string, error) {
	var scripts []string
	for _, dir := range allowedScriptDirs {
		root := filepath.Join(repoRoot, filepath.FromSlash(strings.TrimSuffix(dir, "/")))
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			scripts = append(scripts, filepath.ToSlash(rel))
			return nil
		})
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(scripts)
	return scripts, nil
}

// ValidateStructure checks every entry-level manifest contract without reading
// or executing scripts.
func ValidateStructure(m Manifest) []error {
	var errs []error
	for exampleIndex, example := range m.Examples {
		label := example.Script
		if label == "" {
			label = fmt.Sprintf("examples[%d]", exampleIndex)
		}
		if strings.TrimSpace(example.Description) == "" {
			errs = append(errs, fmt.Errorf("%s: description must not be blank", label))
		}
		if len(example.Commands) == 0 {
			errs = append(errs, fmt.Errorf("%s: commands must not be empty", label))
		}
		seenPaths := make(map[string]struct{}, len(example.Commands))
		for commandIndex, command := range example.Commands {
			if len(command.Path) == 0 {
				errs = append(errs, fmt.Errorf("%s: commands[%d].path must not be empty", label, commandIndex))
			}
			// Each element is one command token. Rejecting embedded whitespace
			// prevents a combined element from looking like multiple tokens in
			// diagnostics or later command-tree comparisons.
			pathValid := len(command.Path) > 0
			for pathIndex, element := range command.Path {
				if strings.TrimSpace(element) == "" {
					errs = append(errs, fmt.Errorf("%s: commands[%d].path[%d] must not be blank", label, commandIndex, pathIndex))
					pathValid = false
				} else if strings.IndexFunc(element, unicode.IsSpace) >= 0 {
					errs = append(errs, fmt.Errorf("%s: commands[%d].path[%d] must not contain whitespace", label, commandIndex, pathIndex))
					pathValid = false
				}
			}
			if pathValid {
				key := commandPathKey(command.Path)
				if _, exists := seenPaths[key]; exists {
					errs = append(errs, fmt.Errorf("%s: duplicate command path %q", label, commandPathText(command.Path)))
				}
				seenPaths[key] = struct{}{}
			}

			seenFlags := make(map[string]struct{}, len(command.Flags))
			for flagIndex, flagName := range command.Flags {
				if strings.TrimSpace(flagName) == "" {
					errs = append(errs, fmt.Errorf("%s: commands[%d].flags[%d] must not be blank", label, commandIndex, flagIndex))
				}
				if _, exists := seenFlags[flagName]; exists {
					errs = append(errs, fmt.Errorf("%s: commands[%d] has duplicate flag %q", label, commandIndex, flagName))
				}
				seenFlags[flagName] = struct{}{}
				if flagIndex > 0 && command.Flags[flagIndex-1] >= flagName {
					errs = append(errs, fmt.Errorf("%s: commands[%d].flags must be strictly sorted", label, commandIndex))
				}
			}
		}

		meaningfulAssertions := 0
		for _, stream := range []struct {
			name       string
			assertions []string
		}{
			{name: "stdout_contains", assertions: example.StdoutContains},
			{name: "stderr_contains", assertions: example.StderrContains},
		} {
			for assertionIndex, assertion := range stream.assertions {
				if strings.TrimSpace(assertion) == "" {
					errs = append(errs, fmt.Errorf("%s: %s[%d] must not be blank", label, stream.name, assertionIndex))
					continue
				}
				meaningfulAssertions++
			}
		}
		if meaningfulAssertions == 0 {
			errs = append(errs, fmt.Errorf("%s: at least one stdout or stderr assertion is required", label))
		}
		if example.MockRequests == nil {
			errs = append(errs, fmt.Errorf("%s: mock_requests is required", label))
		} else if *example.MockRequests < 0 {
			errs = append(errs, fmt.Errorf("%s: mock_requests must be nonnegative", label))
		}
	}
	return errs
}

// ValidateLayout compares the manifest with discovered non-live example files.
func ValidateLayout(m Manifest, discovered []string) []error {
	var errs []error
	discoveredSet := make(map[string]bool, len(discovered))
	for _, script := range discovered {
		discoveredSet[script] = true
	}

	seen := map[string]bool{}
	for _, example := range m.Examples {
		if example.Script == "" {
			errs = append(errs, fmt.Errorf("manifest entry has empty script"))
			continue
		}
		if seen[example.Script] {
			errs = append(errs, fmt.Errorf("%s: duplicate manifest entry", example.Script))
		}
		seen[example.Script] = true
		if !isAllowedScriptPath(example.Script) {
			errs = append(errs, fmt.Errorf("%s: script must be under examples/ci, examples/support, or examples/agents", example.Script))
		}
		if isAllowedScriptPath(example.Script) && !discoveredSet[example.Script] {
			errs = append(errs, fmt.Errorf("%s: manifest entry does not exist", example.Script))
		}
	}
	for _, script := range discovered {
		if !seen[script] {
			errs = append(errs, fmt.Errorf("%s: discovered script missing from manifest", script))
		}
	}
	return errs
}

// ValidateScriptLineCounts rejects scripts above the physical line ceiling.
func ValidateScriptLineCounts(repoRoot string, paths []string) []error {
	var errs []error
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", rel, err))
			continue
		}
		lines := bytes.Count(data, []byte{'\n'})
		if len(data) > 0 && data[len(data)-1] != '\n' {
			lines++
		}
		if lines > maxScriptLines {
			errs = append(errs, fmt.Errorf("%s: script has %d lines, maximum is %d", rel, lines, maxScriptLines))
		}
	}
	return errs
}

// ValidateSafety applies the shared policy to the raw manifest and every
// discovered script, including scripts that are not listed in the manifest.
func ValidateSafety(repoRoot, manifestRel string, discovered []string) []error {
	paths := append([]string{manifestRel}, discovered...)
	var errs []error
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", rel, err))
			continue
		}
		for _, violation := range safetylint.Violations(string(data)) {
			errs = append(errs, fmt.Errorf("%s: %s", rel, violation))
		}
	}
	return errs
}

// ValidateScriptModes fails when a discovered script is not directly
// executable.
func ValidateScriptModes(repoRoot string, paths []string) []error {
	var errs []error
	for _, rel := range paths {
		info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", rel, err))
			continue
		}
		if info.Mode()&0o111 == 0 {
			errs = append(errs, fmt.Errorf("%s: script is not executable", rel))
		}
	}
	return errs
}

// isAllowedScriptPath is a coarse safety gate for manifest paths. The later
// discovered-file comparison still proves that each accepted path exists.
func isAllowedScriptPath(path string) bool {
	if filepath.IsAbs(path) || filepath.Clean(path) != filepath.FromSlash(path) && strings.Contains(path, `\`) {
		return false
	}
	if strings.Contains(path, "..") {
		return false
	}
	for _, prefix := range allowedScriptDirs {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func manifestScripts(m Manifest) []string {
	paths := make([]string, 0, len(m.Examples))
	for _, example := range m.Examples {
		paths = append(paths, example.Script)
	}
	return paths
}

// commandPathKey provides one unambiguous path identity for manifest, catalog,
// and invocation comparisons. Length prefixes keep element boundaries distinct
// even when an element contains punctuation or control bytes.
func commandPathKey(elements []string) string {
	var key strings.Builder
	for _, element := range elements {
		key.WriteString(strconv.Itoa(len(element)))
		key.WriteByte(':')
		key.WriteString(element)
	}
	return key.String()
}

func commandPathText(elements []string) string {
	return strings.Join(elements, " ")
}
