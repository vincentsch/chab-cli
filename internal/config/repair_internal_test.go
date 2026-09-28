package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWritePreservingShapeRejectsAliasToRemovedNode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	original := []byte(`version: 1
current_profile: local
profiles:
  local:
    api_base_url: &shared https://old.example.test/v1
    extension: *shared
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	profilesNode := mappingValue(file.root, "profiles")
	profileNode := mappingValue(profilesNode, "local")
	// Bypass RepairResolvedSelection on purpose to prove the writer has its own
	// final defense against a prepared tree with a removed alias target.
	if !removeMappingPair(profileNode, "api_base_url") {
		t.Fatal("api_base_url pair was not removed")
	}
	profile := file.Profiles["local"]
	profile.APIBaseURL = ""
	file.Profiles["local"] = profile
	meta := file.profileMeta["local"]
	meta.APIBaseURLSet = false
	file.profileMeta["local"] = meta

	if err := WritePreservingShape(path, file); err == nil {
		t.Fatal("WritePreservingShape() error = nil")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("failed alias validation changed config bytes")
	}
}
