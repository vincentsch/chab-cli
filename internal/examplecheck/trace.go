package examplecheck

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
	"unicode"
)

// traceRecord is the value-blind description of one resolved CLI invocation.
// It intentionally contains no arguments, flag values, environment, or output.
type traceRecord struct {
	Path  []string `json:"path"`
	Flags []string `json:"flags"`
}

// loadTrace reads one strict JSON object per nonblank line and returns the raw
// bytes as well so callers can perform an independent protected-value scan.
func loadTrace(path string) ([]traceRecord, []byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("invocation trace: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("invocation trace is not a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		return nil, nil, fmt.Errorf("invocation trace mode = %04o, want 0600", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read invocation trace: %w", err)
	}

	var records []traceRecord
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		// Decode members separately first so missing or null fields cannot be
		// mistaken for valid empty arrays.
		var encoded struct {
			Path  json.RawMessage `json:"path"`
			Flags json.RawMessage `json:"flags"`
		}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&encoded); err != nil {
			return nil, raw, fmt.Errorf("invocation trace line %d is invalid: %w", lineNumber, err)
		}
		var trailing json.RawMessage
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return nil, raw, fmt.Errorf("invocation trace line %d has trailing data", lineNumber)
		}
		if len(encoded.Path) == 0 || bytes.Equal(encoded.Path, []byte("null")) ||
			len(encoded.Flags) == 0 || bytes.Equal(encoded.Flags, []byte("null")) {
			return nil, raw, fmt.Errorf("invocation trace line %d must contain non-null path and flags", lineNumber)
		}
		var record traceRecord
		if err := json.Unmarshal(encoded.Path, &record.Path); err != nil {
			return nil, raw, fmt.Errorf("invocation trace line %d has invalid path: %w", lineNumber, err)
		}
		if err := json.Unmarshal(encoded.Flags, &record.Flags); err != nil {
			return nil, raw, fmt.Errorf("invocation trace line %d has invalid flags: %w", lineNumber, err)
		}
		if err := validateTraceRecord(record); err != nil {
			return nil, raw, fmt.Errorf("invocation trace line %d: %w", lineNumber, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, raw, fmt.Errorf("read invocation trace: %w", err)
	}
	if len(records) == 0 {
		return nil, raw, fmt.Errorf("invocation trace contains no records")
	}
	return records, raw, nil
}

// validateTraceRecord enforces the canonical token and flag shape before a
// trace can participate in manifest comparison.
func validateTraceRecord(record traceRecord) error {
	if len(record.Path) == 0 {
		return fmt.Errorf("path must not be empty")
	}
	for index, element := range record.Path {
		if strings.TrimSpace(element) == "" {
			return fmt.Errorf("path[%d] must not be blank", index)
		}
		if strings.IndexFunc(element, unicode.IsSpace) >= 0 {
			return fmt.Errorf("path[%d] must not contain whitespace", index)
		}
	}
	for index, flagName := range record.Flags {
		if strings.TrimSpace(flagName) == "" {
			return fmt.Errorf("flags[%d] must not be blank", index)
		}
		if index > 0 && record.Flags[index-1] >= flagName {
			return fmt.Errorf("flags must be strictly sorted and unique")
		}
	}
	return nil
}

// compareTrace reports drift in both directions. Repeated calls to the same
// command are merged so the observed flag set is the union across the script.
func compareTrace(example Example, observed []traceRecord) []error {
	declared, declaredPaths := commandFlagMap(example.Commands)
	observedUses := make(map[string]map[string]struct{})
	observedPaths := make(map[string]string)
	// A script may call one command several times with different flags. Merge
	// those records before comparing them with the manifest's declared union.
	for _, record := range observed {
		key := commandPathKey(record.Path)
		flags := observedUses[key]
		if flags == nil {
			flags = make(map[string]struct{})
			observedUses[key] = flags
			observedPaths[key] = commandPathText(record.Path)
		}
		for _, flagName := range record.Flags {
			flags[flagName] = struct{}{}
		}
	}

	var errs []error
	// Check observed data first, then walk declarations separately so missing
	// commands and flags are reported rather than hidden by a one-way subset.
	for _, key := range sortedPathKeys(observedUses, observedPaths) {
		path := observedPaths[key]
		declaredFlags, exists := declared[key]
		if !exists {
			errs = append(errs, fmt.Errorf("observed command %q is not declared", path))
			continue
		}
		for _, flagName := range sortedSetKeys(observedUses[key]) {
			if _, exists := declaredFlags[flagName]; !exists {
				errs = append(errs, fmt.Errorf("observed flag %q for command %q is not declared", "--"+flagName, path))
			}
		}
	}
	for _, key := range sortedPathKeys(declared, declaredPaths) {
		path := declaredPaths[key]
		observedFlags, exists := observedUses[key]
		if !exists {
			errs = append(errs, fmt.Errorf("declared command %q was not observed", path))
			continue
		}
		for _, flagName := range sortedSetKeys(declared[key]) {
			if _, exists := observedFlags[flagName]; !exists {
				errs = append(errs, fmt.Errorf("declared flag %q for command %q was not observed", "--"+flagName, path))
			}
		}
	}
	return errs
}

// commandFlagMap returns flag sets keyed by the canonical path plus separate
// display labels for readable, deterministic diagnostics.
func commandFlagMap(uses []CommandUse) (map[string]map[string]struct{}, map[string]string) {
	result := make(map[string]map[string]struct{}, len(uses))
	paths := make(map[string]string, len(uses))
	for _, use := range uses {
		key := commandPathKey(use.Path)
		flags := result[key]
		if flags == nil {
			flags = make(map[string]struct{}, len(use.Flags))
			result[key] = flags
			paths[key] = commandPathText(use.Path)
		}
		for _, flagName := range use.Flags {
			flags[flagName] = struct{}{}
		}
	}
	return result, paths
}

func sortedPathKeys[V any](values map[string]V, paths map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		return paths[keys[left]] < paths[keys[right]]
	})
	return keys
}

func sortedSetKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
