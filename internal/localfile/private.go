package localfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// ReservedPrivateFile is an exclusively created private file that has been
// reserved before a one-time secret request is sent.
type ReservedPrivateFile struct {
	path   string
	file   *os.File
	closed bool
}

// ReservePrivate creates path with O_EXCL and 0600 permissions. The caller owns
// either Commit or Abort; no payload is written until Commit is called.
func ReservePrivate(path string) (*ReservedPrivateFile, error) {
	if path == "" {
		return nil, fmt.Errorf("private output path must not be empty")
	}
	if err := privateOutputSupported(); err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return &ReservedPrivateFile{path: path, file: file}, nil
}

// Path returns the reserved output path.
func (r *ReservedPrivateFile) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

// Commit writes, syncs, and closes the reserved file.
func (r *ReservedPrivateFile) Commit(data []byte) (err error) {
	if r == nil || r.file == nil || r.closed {
		return fmt.Errorf("private output is not open")
	}
	defer func() {
		if err != nil && !r.closed {
			_ = r.file.Close()
			r.closed = true
		}
	}()
	if _, err := r.file.Write(data); err != nil {
		return err
	}
	if err := r.file.Sync(); err != nil {
		return err
	}
	if err := r.file.Close(); err != nil {
		return err
	}
	r.closed = true
	syncDirBestEffort(filepath.Dir(r.path))
	return nil
}

// Abort closes the reservation and optionally removes the empty output file.
func (r *ReservedPrivateFile) Abort(remove bool) error {
	if r == nil || r.file == nil || r.closed {
		return nil
	}
	err := r.file.Close()
	r.closed = true
	if remove {
		if rmErr := os.Remove(r.path); err == nil {
			err = rmErr
		}
	}
	return err
}
