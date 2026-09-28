package localfile

import (
	"io"
	"os"
	"path/filepath"
)

// CreateExclusive streams into a private temporary file and publishes only a
// complete file, without overwriting an existing file or following its symlink.
// The parent directory must already exist. Hard-link publication is atomic and
// fails safely on filesystems without support rather than weakening no-clobber.
func CreateExclusive(path string, write func(io.Writer) error) error {
	if _, err := os.Lstat(path); err == nil {
		return &os.PathError{Op: "create", Path: path, Err: os.ErrExist}
	} else if !os.IsNotExist(err) {
		return err
	}
	file, tmp, err := createTemp(filepath.Dir(path), filepath.Base(path), 0600)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	defer file.Close()
	if err := write(file); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Link(tmp, path); err != nil {
		return err
	}
	syncDirBestEffort(filepath.Dir(path))
	return nil
}
