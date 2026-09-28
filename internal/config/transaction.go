package config

import (
	"errors"
	"os"

	"github.com/vincentsch/chab-cli/internal/localfile"
)

// RollbackSnapshot captures the on-disk config file state before a multi-file
// write. It stores raw bytes so rollback preserves comments, unknown YAML, and
// omitted-versus-explicit known-field shape.
type RollbackSnapshot struct {
	path   string
	exists bool
	data   []byte
}

// CaptureRollbackSnapshot records the current config file bytes. The loaded
// file is accepted so callers can use the same existence decision as Load.
func CaptureRollbackSnapshot(path string, file *File) (RollbackSnapshot, error) {
	snapshot := RollbackSnapshot{path: path}
	if file == nil || !file.Exists() {
		// Remember that the login attempt created the file so rollback removes
		// it instead of writing an empty config.
		return snapshot, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return RollbackSnapshot{}, newError(ErrMalformedConfig, path, "", "", "could not snapshot config file", err)
	}
	snapshot.exists = true
	snapshot.data = append([]byte(nil), data...)
	return snapshot, nil
}

// RestoreRollbackSnapshot restores or removes the config file to match the
// captured state.
func RestoreRollbackSnapshot(snapshot RollbackSnapshot) error {
	if snapshot.path == "" {
		return newError(ErrMalformedConfig, "", "", "path", "must not be empty", nil)
	}
	if !snapshot.exists {
		// The original state was "no file"; best-effort removal restores that
		// state even when a later write created parent directories.
		if err := os.Remove(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return newError(ErrMalformedConfig, snapshot.path, "", "", "could not remove config file during rollback", err)
		}
		return nil
	}
	if err := localfile.AtomicWrite(snapshot.path, snapshot.data, 0o700, 0o644); err != nil {
		return newError(ErrMalformedConfig, snapshot.path, "", "", "could not restore config file during rollback", err)
	}
	return nil
}
