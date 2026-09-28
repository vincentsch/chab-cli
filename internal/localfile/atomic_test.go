package localfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vincentsch/chab-cli/internal/localfile"
)

func TestAtomicWriteRemovesTempFileOnReplaceFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	// Replacing a directory with a file fails after AtomicWrite has created
	// the same-directory temp file, which exercises the cleanup path.
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	err := localfile.AtomicWrite(target, []byte("payload"), 0o700, 0o600)
	if err == nil {
		t.Fatalf("AtomicWrite() error = nil, want replace failure")
	}

	matches, globErr := filepath.Glob(filepath.Join(dir, ".target.tmp-*"))
	if globErr != nil {
		t.Fatalf("Glob() error = %v", globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("AtomicWrite left temp files after failure: %#v", matches)
	}
}
