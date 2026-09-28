//go:build !windows

package localfile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCreateTempAppliesFinalModeBeforePayload(t *testing.T) {
	for _, test := range []struct {
		name  string
		umask int
		mode  os.FileMode
	}{
		{name: "secret under permissive umask", umask: 0, mode: 0o600},
		{name: "config under restrictive umask", umask: 0o077, mode: 0o644},
	} {
		t.Run(test.name, func(t *testing.T) {
			oldUmask := syscall.Umask(test.umask)
			t.Cleanup(func() {
				syscall.Umask(oldUmask)
			})

			dir := t.TempDir()
			file, path, err := createTemp(dir, "state", test.mode)
			if err != nil {
				t.Fatalf("createTemp() error = %v", err)
			}
			t.Cleanup(func() {
				_ = file.Close()
				_ = os.Remove(path)
			})

			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("Stat() error = %v", err)
			}
			if got := info.Mode().Perm(); got != test.mode {
				t.Fatalf("temp mode before payload = %04o, want %04o", got, test.mode)
			}
			if matches, err := filepath.Glob(filepath.Join(dir, ".state.tmp-*")); err != nil || len(matches) != 1 {
				t.Fatalf("temp files = %#v, %v", matches, err)
			}
		})
	}
}
