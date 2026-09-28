package localfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestExclusiveDownloadDoesNotClobberOrPublishPartialFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mail.eml")
	if err := CreateExclusive(path, func(w io.Writer) error { io.WriteString(w, "partial"); return errors.New("interrupted") }); err == nil {
		t.Fatal("failed write succeeded")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("partial files: %v", entries)
	}
	if err := CreateExclusive(path, func(w io.Writer) error { _, err := io.WriteString(w, "complete"); return err }); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := CreateExclusive(path, func(w io.Writer) error { called = true; return nil }); !errors.Is(err, os.ErrExist) || called {
		t.Fatalf("clobbered existing file: %v", err)
	}
	bytes, _ := os.ReadFile(path)
	if string(bytes) != "complete" {
		t.Fatal("existing content changed")
	}
}
