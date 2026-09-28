package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestExpectedArchivesCoversSupportedMatrix(t *testing.T) {
	got := keys(expectedArchives("0.0.0"))
	sort.Strings(got)
	want := []string{
		"chab_0.0.0_agent.zip",
		"chab_0.0.0_darwin_amd64.tar.gz",
		"chab_0.0.0_darwin_arm64.tar.gz",
		"chab_0.0.0_linux_amd64.tar.gz",
		"chab_0.0.0_linux_arm64.tar.gz",
		"chab_0.0.0_windows_amd64.zip",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected archive set = %v, want %v", got, want)
	}

	archives := expectedArchives("0.0.0")
	if archives["chab_0.0.0_windows_amd64.zip"].binary != "chab.exe" {
		t.Fatalf("windows archive binary = %q, want chab.exe", archives["chab_0.0.0_windows_amd64.zip"].binary)
	}
	for name, meta := range archives {
		if strings.HasSuffix(name, ".zip") != (meta.format == "zip") {
			t.Fatalf("%s format = %q", name, meta.format)
		}
	}
}

func TestAssertArchiveEntriesRequiresOneSafeTopLevelBinary(t *testing.T) {
	tests := []struct {
		name    string
		entries []archiveEntry
		wantErr string
	}{
		{
			name: "regular top-level binary",
			entries: []archiveEntry{
				{name: "chab", regular: true},
				{name: "README.md", regular: true},
			},
		},
		{
			name: "absolute path",
			entries: []archiveEntry{
				{name: "/chab", regular: true},
			},
			wantErr: "unsafe member path",
		},
		{
			name: "path traversal",
			entries: []archiveEntry{
				{name: "docs/../chab", regular: true},
			},
			wantErr: "unsafe member path",
		},
		{
			name: "link member",
			entries: []archiveEntry{
				{name: "chab", regular: false, link: true},
			},
			wantErr: "link member",
		},
		{
			name: "non-regular member",
			entries: []archiveEntry{
				{name: "chab", regular: false},
			},
			wantErr: "non-regular member",
		},
		{
			name: "nested candidate",
			entries: []archiveEntry{
				{name: "chab", regular: true},
				{name: "bin/chab", regular: true},
			},
			wantErr: "exactly one regular top-level chab binary",
		},
		{
			name: "missing top-level binary",
			entries: []archiveEntry{
				{name: "bin/chab", regular: true},
			},
			wantErr: "exactly one regular top-level chab binary",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := assertArchiveEntries("archive.tar.gz", tc.entries, "chab")
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("assertArchiveEntries() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("assertArchiveEntries() error = nil, want failure")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("assertArchiveEntries() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestUnitTestsDoNotInvokeSmokeRunner(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, "_test.go") {
			continue
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		ast.Inspect(file, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if !ok || ident.Name != "run" {
				return true
			}

			t.Fatalf("%s references run(); unit tests must cover pure helpers only", fset.Position(ident.Pos()))
			return false
		})
	}
}
