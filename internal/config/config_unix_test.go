//go:build !windows

package config_test

import (
	"path/filepath"
	"syscall"
	"testing"

	"github.com/vincentsch/chab-cli/internal/config"
)

func TestConfigWritePermissionsUnderRestrictiveUmaskAndSpaces(t *testing.T) {
	// The path shape exercises ordinary filesystem handling while the umask
	// keeps this focused on the writer's explicit modes.
	oldUmask := syscall.Umask(0o077)
	t.Cleanup(func() {
		syscall.Umask(oldUmask)
	})

	path := filepath.Join(t.TempDir(), "dir with spaces", "config file.yml")
	file := &config.File{}
	if err := config.UpsertProfile(file, "local", config.Profile{BaseURL: "https://file.example.test"}); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}
	if err := config.SetCurrentProfile(file, "local"); err != nil {
		t.Fatalf("SetCurrentProfile() error = %v", err)
	}
	if err := config.Write(path, file); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	assertMode(t, filepath.Dir(path), 0o700)
	assertMode(t, path, 0o644)
}

func TestConfigWritePermissionsUnderPermissiveUmask(t *testing.T) {
	oldUmask := syscall.Umask(0)
	t.Cleanup(func() {
		syscall.Umask(oldUmask)
	})

	path := filepath.Join(t.TempDir(), "config", "config.yml")
	file := &config.File{}
	if err := config.UpsertProfile(file, "local", config.Profile{BaseURL: "https://file.example.test"}); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}
	if err := config.SetCurrentProfile(file, "local"); err != nil {
		t.Fatalf("SetCurrentProfile() error = %v", err)
	}
	if err := config.Write(path, file); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	assertMode(t, filepath.Dir(path), 0o700)
	assertMode(t, path, 0o644)
}
