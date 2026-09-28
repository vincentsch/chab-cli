package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/config"
)

func TestRepairResolvedSelectionBuildsMinimalMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	changed, err := config.RepairResolvedSelection(file, config.RepairSelection{
		Profile:          "staging",
		BaseURL:          "https://staging.example.test/",
		APIBaseURL:       "https://staging.example.test/v1",
		APIBaseURLSource: config.RuntimeValueSourceDerived,
	})
	if err != nil || !changed {
		t.Fatalf("RepairResolvedSelection() = %t, %v", changed, err)
	}
	if err := config.WritePreservingShape(path, file); err != nil {
		t.Fatalf("WritePreservingShape() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := []byte("version: 1\ncurrent_profile: staging\nprofiles:\n    staging:\n        base_url: https://staging.example.test\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("minimal config =\n%s\nwant:\n%s", got, want)
	}
}

func TestRepairResolvedSelectionChangesOnlySelectedProfileScalar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	original := []byte(`version: 1
current_profile: 'old'
profiles:
  old:
    extension: keep-old
  target:
    extension: keep-target
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	changed, err := config.RepairResolvedSelection(file, config.RepairSelection{
		Profile:          "target",
		BaseURL:          config.LegacyImplicitBaseURL,
		APIBaseURL:       "http://localhost/v1",
		APIBaseURLSource: config.RuntimeValueSourceDerived,
	})
	if err != nil || !changed {
		t.Fatalf("RepairResolvedSelection() = %t, %v", changed, err)
	}
	if err := config.WritePreservingShape(path, file); err != nil {
		t.Fatalf("WritePreservingShape() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := []byte(`version: 1
current_profile: 'target'
profiles:
    old:
        extension: keep-old
    target:
        extension: keep-target
`)
	if !bytes.Equal(got, want) {
		t.Fatalf("selection-only config =\n%s\nwant:\n%s", got, want)
	}
}

func TestRepairResolvedSelectionPreservesShapeAndRepairsOnlyNeededFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	original := []byte(`# root comment
version: 1
unknown_top: keep
current_profile: old
profiles:
  old:
    base_url: https://old.example.test
    extension: keep-old
  staging:
    # base comment
    base_url: 'https://staging.example.test/'
    api_base_url: "https://stale.example.test/v1"
    locale: ""
    extension: keep-staging
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	changed, err := config.RepairResolvedSelection(file, config.RepairSelection{
		Profile:          "staging",
		BaseURL:          "https://staging.example.test",
		APIBaseURL:       "https://staging.example.test/v1",
		APIBaseURLSource: config.RuntimeValueSourceDerived,
	})
	if err != nil || !changed {
		t.Fatalf("RepairResolvedSelection() = %t, %v", changed, err)
	}
	if err := config.WritePreservingShape(path, file); err != nil {
		t.Fatalf("WritePreservingShape() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(got)
	for _, want := range []string{
		"# root comment",
		"unknown_top: keep",
		"current_profile: staging",
		"extension: keep-old",
		"# base comment",
		"base_url: 'https://staging.example.test/'",
		`locale: ""`,
		"extension: keep-staging",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("preserved config missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "api_base_url:") {
		t.Fatalf("stale derived API URL was retained:\n%s", text)
	}
	if strings.Index(text, "unknown_top:") > strings.Index(text, "current_profile:") {
		t.Fatalf("root mapping order changed:\n%s", text)
	}
}

func TestRepairResolvedSelectionKeepsReferencedAPIAnchorValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	original := []byte(`version: 1
current_profile: local
profiles:
  local:
    base_url: https://old.example.test
    api_base_url: &shared https://old.example.test/v1
    extension: *shared
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	changed, err := config.RepairResolvedSelection(file, config.RepairSelection{
		Profile:          "local",
		BaseURL:          "https://new.example.test",
		APIBaseURL:       "https://new.example.test/v1",
		APIBaseURLSource: config.RuntimeValueSourceDerived,
	})
	if err != nil || !changed {
		t.Fatalf("RepairResolvedSelection() = %t, %v", changed, err)
	}
	if err := config.WritePreservingShape(path, file); err != nil {
		t.Fatalf("WritePreservingShape() error = %v", err)
	}
	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load(repaired) error = %v", err)
	}
	profile := reloaded.Profiles["local"]
	if profile.BaseURL != "https://new.example.test" ||
		profile.APIBaseURL != "https://new.example.test/v1" {
		t.Fatalf("reloaded profile = %#v", profile)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		"api_base_url: &shared https://new.example.test/v1",
		"extension: *shared",
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("repaired config missing %q:\n%s", want, got)
		}
	}
}

func TestRepairResolvedSelectionRetainsEquivalentExplicitAPIAndNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	original := []byte(`version: 1
current_profile: local
profiles:
  local:
    base_url: https://www.chab.ai
    api_base_url: https://www.chab.ai/v1
    locale: ""
    extension: keep
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	changed, err := config.RepairResolvedSelection(file, config.RepairSelection{
		Profile:          "local",
		BaseURL:          config.DefaultBaseURL,
		APIBaseURL:       config.DefaultAPIBaseURL,
		APIBaseURLSource: config.RuntimeValueSourceDerived,
	})
	if err != nil || changed {
		t.Fatalf("RepairResolvedSelection() = %t, %v; want no-op", changed, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("no-op changed bytes:\n%s", got)
	}
}

func TestRepairResolvedSelectionPersistsExplicitOverridesForNewProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	original := []byte(`version: 1
current_profile: old
profiles:
  old:
    extension: keep
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	changed, err := config.RepairResolvedSelection(file, config.RepairSelection{
		Profile:          "new",
		BaseURL:          "https://new.example.test",
		APIBaseURL:       "https://api.example.test/v1",
		APIBaseURLSource: config.RuntimeValueSourceFlag,
		Locale:           "de",
	})
	if err != nil || !changed {
		t.Fatalf("RepairResolvedSelection() = %t, %v", changed, err)
	}
	if err := config.WritePreservingShape(path, file); err != nil {
		t.Fatalf("WritePreservingShape() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		"extension: keep",
		"new:",
		"base_url: https://new.example.test",
		"api_base_url: https://api.example.test/v1",
		"locale: de",
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("config missing %q:\n%s", want, got)
		}
	}
	if bytes.Contains(got, []byte("default_output")) || bytes.Contains(got, []byte("defaults:")) {
		t.Fatalf("repair materialized unrelated defaults:\n%s", got)
	}
}

func TestRepairResolvedSelectionLocalePresenceSemantics(t *testing.T) {
	for _, test := range []struct {
		name        string
		localeLine  string
		desired     string
		wantChanged bool
		wantLine    string
	}{
		{name: "omitted empty", desired: "", wantLine: ""},
		{name: "explicit empty", localeLine: `    locale: ""` + "\n", desired: "", wantLine: `locale: ""`},
		{name: "omitted nonempty", desired: "de", wantChanged: true, wantLine: "locale: de"},
		{name: "explicit mismatch", localeLine: `    locale: "fr"` + "\n", desired: "de", wantChanged: true, wantLine: `locale: "de"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			original := []byte("version: 1\ncurrent_profile: local\nprofiles:\n  local:\n" + test.localeLine + "    extension: keep\n")
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			file, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			changed, err := config.RepairResolvedSelection(file, config.RepairSelection{
				Profile:          "local",
				BaseURL:          config.LegacyImplicitBaseURL,
				APIBaseURL:       "http://localhost/v1",
				APIBaseURLSource: config.RuntimeValueSourceDerived,
				Locale:           test.desired,
			})
			if err != nil || changed != test.wantChanged {
				t.Fatalf("RepairResolvedSelection() = %t, %v; want changed=%t", changed, err, test.wantChanged)
			}
			if !changed {
				got, readErr := os.ReadFile(path)
				if readErr != nil {
					t.Fatalf("ReadFile() error = %v", readErr)
				}
				if !bytes.Equal(got, original) {
					t.Fatal("locale no-op changed file bytes")
				}
				return
			}
			if err := config.WritePreservingShape(path, file); err != nil {
				t.Fatalf("WritePreservingShape() error = %v", err)
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("ReadFile() error = %v", readErr)
			}
			if !bytes.Contains(got, []byte(test.wantLine)) ||
				!bytes.Contains(got, []byte("extension: keep")) {
				t.Fatalf("locale repair =\n%s", got)
			}
		})
	}
}

func TestRepairResolvedSelectionPersistsEnvironmentAPIOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	changed, err := config.RepairResolvedSelection(file, config.RepairSelection{
		Profile:          "environment",
		BaseURL:          "https://product.example.test",
		APIBaseURL:       "https://api.example.test/root",
		APIBaseURLSource: config.RuntimeValueSourceEnv,
	})
	if err != nil || !changed {
		t.Fatalf("RepairResolvedSelection() = %t, %v", changed, err)
	}
	if err := config.WritePreservingShape(path, file); err != nil {
		t.Fatalf("WritePreservingShape() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		"base_url: https://product.example.test",
		"api_base_url: https://api.example.test/root",
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("environment API repair missing %q:\n%s", want, got)
		}
	}
}

func TestRepairResolvedSelectionValidatesDesiredValuesBeforeMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	original := []byte(`version: 1
current_profile: local
profiles:
  local:
    extension: keep
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	changed, err := config.RepairResolvedSelection(file, config.RepairSelection{
		Profile:          "other",
		BaseURL:          "https://example.test",
		APIBaseURL:       "https://different.example.test/v1",
		APIBaseURLSource: config.RuntimeValueSourceDerived,
	})
	if err == nil || changed {
		t.Fatalf("RepairResolvedSelection() = %t, %v", changed, err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("invalid desired selection changed file bytes")
	}
}

func TestWritePreservingShapeRejectsInvalidUnrelatedKnownFieldBeforeWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	original := []byte(`version: 1
current_profile: target
profiles:
  target:
    base_url: https://target.example.test
  unrelated:
    locale: fr
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	file, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	changed, err := config.RepairResolvedSelection(file, config.RepairSelection{
		Profile:          "target",
		BaseURL:          "https://target.example.test",
		APIBaseURL:       "https://api.example.test/v1",
		APIBaseURLSource: config.RuntimeValueSourceFlag,
	})
	if err != nil || !changed {
		t.Fatalf("RepairResolvedSelection() = %t, %v", changed, err)
	}
	if err := config.WritePreservingShape(path, file); err == nil {
		t.Fatal("WritePreservingShape() error = nil")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("failed validation changed bytes:\n%s", got)
	}
}

func FuzzRepairResolvedSelectionPreservesWritableTrees(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("version: 1\ncurrent_profile: local\nprofiles:\n  local: {}\n"),
		[]byte("# comment\nversion: 1\ncurrent_profile: old\nprofiles:\n  old:\n    extension: keep\n"),
		[]byte("version: 1\ncurrent_profile: local\nprofiles:\n  local:\n    api_base_url: &shared http://localhost/v1\n    extension: *shared\n"),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, original []byte) {
		if len(original) > 8192 {
			t.Skip()
		}
		path := filepath.Join(t.TempDir(), "config.yml")
		if err := os.WriteFile(path, original, 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		file, err := config.Load(path)
		if err != nil {
			return
		}
		changed, err := config.RepairResolvedSelection(file, config.RepairSelection{
			Profile:          "local",
			BaseURL:          config.DefaultBaseURL,
			APIBaseURL:       config.DefaultAPIBaseURL,
			APIBaseURLSource: config.RuntimeValueSourceDerived,
		})
		if err != nil {
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("ReadFile() error = %v", readErr)
			}
			if !bytes.Equal(got, original) {
				t.Fatal("rejected repair changed file bytes")
			}
			return
		}
		if !changed {
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("ReadFile() error = %v", readErr)
			}
			if !bytes.Equal(got, original) {
				t.Fatal("no-op repair changed file bytes")
			}
			return
		}
		if err := config.WritePreservingShape(path, file); err != nil {
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("ReadFile() error = %v", readErr)
			}
			if !bytes.Equal(got, original) {
				t.Fatal("failed preserving write changed file bytes")
			}
			return
		}
		if _, err := config.Load(path); err != nil {
			t.Fatalf("Load(repaired) error = %v", err)
		}
	})
}
