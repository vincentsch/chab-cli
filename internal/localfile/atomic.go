// Package localfile contains local filesystem helpers shared by config and auth
// persistence. It has no CLI, Cobra, environment, or network dependencies.
package localfile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// AtomicWrite writes data to a same-directory temporary file created with the
// final file mode before any bytes are written, syncs and closes it, then
// atomically renames it over path.
func AtomicWrite(path string, data []byte, dirMode, fileMode os.FileMode) error {
	return writeTempThenPublish(path, dirMode, fileMode, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	}, func(tmpPath string) error {
		return os.Rename(tmpPath, path)
	})
}

func writeTempThenPublish(path string, dirMode, fileMode os.FileMode, write func(io.Writer) error, publish func(string) error) error {
	dir := filepath.Dir(path)
	createdDir := false
	if _, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		createdDir = true
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	if createdDir {
		// MkdirAll is still subject to umask. Chmod restores the intended mode
		// before any file payload is created in the directory.
		if err := os.Chmod(dir, dirMode); err != nil {
			return err
		}
	}

	tmp, tmpPath, err := createTemp(dir, filepath.Base(path), fileMode)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			// Failed writes should not leave partial temp files behind. Remove is
			// best effort because the original error is more useful to callers.
			_ = os.Remove(tmpPath)
		}
	}()

	// The temp file is created with the final mode before this point, so secret
	// auth data is never written through a broader permission window.
	if err := write(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := publish(tmpPath); err != nil {
		return err
	}
	keep = true
	// Directory sync is best effort for portability; the data file itself was
	// already synced before the rename.
	syncDirBestEffort(dir)
	return nil
}

// createTemp uses O_EXCL with random names so concurrent writers do not share
// a temporary file.
func createTemp(dir, base string, mode os.FileMode) (*os.File, string, error) {
	var lastErr error
	for i := 0; i < 100; i++ {
		suffix, err := randomSuffix()
		if err != nil {
			return nil, "", err
		}
		path := filepath.Join(dir, "."+base+".tmp-"+suffix)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err == nil {
			// OpenFile honors umask; Chmod narrows or restores the exact final
			// mode before the caller writes any bytes.
			if err := file.Chmod(mode); err != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return nil, "", err
			}
			return file, path, nil
		}
		if errors.Is(err, os.ErrExist) {
			lastErr = err
			continue
		}
		return nil, "", err
	}
	return nil, "", lastErr
}

func randomSuffix() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// syncDirBestEffort asks the filesystem to persist the rename metadata where
// supported. Some platforms/filesystems reject directory fsync, so failures are
// intentionally ignored.
func syncDirBestEffort(dir string) {
	handle, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = handle.Sync()
	_ = handle.Close()
}
