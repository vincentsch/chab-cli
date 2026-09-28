package authcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/config"
)

func TestRollbackWarningWhenConfigRestoreFails(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ak_path%7Cpath-secret", "config.yml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := []byte("version: 1\ncurrent_profile: old\nprofiles: {}\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	snapshot, err := config.CaptureRollbackSnapshot(configPath, cfg)
	if err != nil {
		t.Fatalf("CaptureRollbackSnapshot() error = %v", err)
	}
	if err := os.Remove(configPath); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if err := os.Mkdir(configPath, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)
	rollbackLoginConfig(cmd, config.Runtime{ConfigPath: configPath}, "ak_profile|profile-secret", snapshot)

	got := stderr.String()
	for _, want := range []string{"Warning: login updated config", "rollback also failed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("warning missing %q:\n%s", want, got)
		}
	}
	for _, leaked := range []string{"path-secret", "profile-secret"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("warning leaked %q:\n%s", leaked, got)
		}
	}
}
