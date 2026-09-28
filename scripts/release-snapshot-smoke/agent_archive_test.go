package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestAgentBundleRejectsUninstallableOrDriftedArtifacts(t *testing.T) {
	const skill = "---\nname: chab\n---\nStandalone workflow.\n"
	for _, tc := range []struct {
		name    string
		members []string
		body    string
		link    bool
		valid   bool
	}{
		{"standalone", []string{"chab/SKILL.md"}, skill, false, true},
		{"missing", nil, skill, false, false},
		{"duplicate", []string{"chab/SKILL.md", "chab/SKILL.md"}, skill, false, false},
		{"extra dependency", []string{"chab/SKILL.md", "chab/commands.md"}, skill, false, false},
		{"wrong directory", []string{"SKILL.md"}, skill, false, false},
		{"traversal", []string{"../chab/SKILL.md"}, skill, false, false},
		{"symlink", []string{"chab/SKILL.md"}, skill, true, false},
		{"stale content", []string{"chab/SKILL.md"}, "old skill", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "agent.zip")
			f, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			zw := zip.NewWriter(f)
			for _, name := range tc.members {
				h := &zip.FileHeader{Name: name, Method: zip.Deflate}
				h.SetMode(0o644)
				if tc.link {
					h.SetMode(os.ModeSymlink | 0o777)
				}
				w, err := zw.CreateHeader(h)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Fatal(err)
				}
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			if err := verifyAgentArchive(archive, []byte(skill)); (err == nil) != tc.valid {
				t.Fatalf("verifyAgentArchive = %v; valid = %v", err, tc.valid)
			}
		})
	}
}
