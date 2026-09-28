package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
)

// Verify the actual downloadable skill, including its installable directory and
// exact standalone contents. No maintainer checkout files belong in this bundle.
func verifyAgentArchive(archivePath string, expected []byte) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	if len(zr.File) != 1 {
		return fmt.Errorf("agent bundle must contain exactly one standalone skill")
	}
	f := zr.File[0]
	if f.Name != "chab/SKILL.md" || !f.Mode().IsRegular() {
		return fmt.Errorf("agent bundle has unexpected member %q", f.Name)
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, int64(len(expected))+1))
	if err != nil {
		return err
	}
	if len(expected) == 0 || !bytes.Equal(data, expected) {
		return fmt.Errorf("agent bundle skill differs from the checked standalone entrypoint")
	}
	return nil
}
