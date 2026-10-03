//go:build !windows

package localfile

import (
	"os"
)

func openExclusiveTemp(path string, mode os.FileMode) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return nil, err
	}
	// OpenFile honors umask; restore the exact final mode before any bytes.
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return file, nil
}
