//go:build !windows

package auth_test

import (
	"path/filepath"
	"syscall"
	"testing"

	"github.com/vincentsch/chab-cli/internal/auth"
)

func TestAuthWritePermissionsUnderRestrictiveUmaskAndSpaces(t *testing.T) {
	// The path shape exercises ordinary filesystem handling while the umask
	// keeps this focused on the writer's explicit modes.
	oldUmask := syscall.Umask(0o077)
	t.Cleanup(func() {
		syscall.Umask(oldUmask)
	})

	path := filepath.Join(t.TempDir(), "dir with spaces", "auth file.json")
	file := &auth.File{}
	if err := auth.PutProfile(file, "local", sampleRecord("ak_umask|secret")); err != nil {
		t.Fatalf("PutProfile() error = %v", err)
	}
	if err := auth.Write(path, file); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	assertMode(t, filepath.Dir(path), 0o700)
	assertMode(t, path, 0o600)
}

func TestAuthWritePermissionsUnderPermissiveUmask(t *testing.T) {
	oldUmask := syscall.Umask(0)
	t.Cleanup(func() {
		syscall.Umask(oldUmask)
	})

	path := filepath.Join(t.TempDir(), "auth", "auth.json")
	file := &auth.File{}
	if err := auth.PutProfile(file, "local", sampleRecord("ak_umask_permissive|secret")); err != nil {
		t.Fatalf("PutProfile() error = %v", err)
	}
	if err := auth.Write(path, file); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	assertMode(t, filepath.Dir(path), 0o700)
	assertMode(t, path, 0o600)
}
