package localfile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReservePrivateCommitsExclusivePrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "secret.json")
	reserved, err := ReservePrivate(path)
	if runtime.GOOS == "windows" {
		if err == nil || !strings.Contains(err.Error(), "owner-only ACLs") {
			t.Fatalf("ReservePrivate() error = %v, want owner-only ACL failure", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("ReservePrivate() error = %v", err)
	}
	if err := reserved.Commit([]byte("{\"ok\":true}\n")); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("mode = %o, want 600", mode)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "{\"ok\":true}\n" {
		t.Fatalf("data = %q", data)
	}
	if _, err := ReservePrivate(path); !errors.Is(err, os.ErrExist) {
		t.Fatalf("second ReservePrivate() error = %v, want exists", err)
	}
}

func TestReservePrivateAbortCanRemoveReservation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows private output fails before reserving files")
	}
	path := filepath.Join(t.TempDir(), "secret.json")
	reserved, err := ReservePrivate(path)
	if err != nil {
		t.Fatalf("ReservePrivate() error = %v", err)
	}
	if err := reserved.Abort(true); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat() error = %v, want not exist", err)
	}
}
