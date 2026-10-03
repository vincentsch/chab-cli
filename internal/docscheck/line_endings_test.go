package docscheck

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPinnedSourceFilesStayLFInCheckout protects raw-byte provenance and skill
// manifests from a Windows checkout with core.autocrlf=true. Keep the file list
// sourced from the manifests so a newly pinned file inherits this check.
func TestPinnedSourceFilesStayLFInCheckout(t *testing.T) {
	root := findRepoRoot(t)
	type fixture struct {
		File   string `json:"file"`
		SHA256 string `json:"sha256"`
	}
	var provenance struct {
		Fixtures []fixture `json:"fixtures"`
	}
	data, err := os.ReadFile(filepath.Join(root, "internal/chabcontract/testdata/provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &provenance); err != nil {
		t.Fatal(err)
	}
	if len(provenance.Fixtures) == 0 {
		t.Fatal("contract provenance has no fixtures")
	}
	pinned := map[string]string{}
	for _, entry := range provenance.Fixtures {
		if entry.File == "" || entry.SHA256 == "" {
			t.Fatalf("incomplete contract fixture: %+v", entry)
		}
		pinned["internal/chabcontract/testdata/"+entry.File] = entry.SHA256
	}

	const manifest = ".chab-agent-skill/manifest.sha256"
	data, err = os.ReadFile(filepath.Join(root, manifest))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("malformed skill manifest line %q", line)
		}
		pinned[fields[1]] = fields[0]
	}
	if len(pinned) < len(provenance.Fixtures)+1 {
		t.Fatal("skill manifest has no pinned files")
	}

	// The manifest itself is not listed within its own checksum lines.
	paths := make([]string, 0, len(pinned)+1)
	for rel := range pinned {
		paths = append(paths, rel)
	}
	paths = append(paths, manifest, "scripts/smoke-release-no-secret.sh")
	for _, rel := range paths {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(got, []byte("\r\n")) {
			t.Errorf("%s was checked out with CRLF", rel)
		}
		if expected, ok := pinned[rel]; ok {
			sum := sha256.Sum256(got)
			if hex.EncodeToString(sum[:]) != expected {
				t.Errorf("%s does not match its pinned raw-byte SHA-256", rel)
			}
		}
		cmd := exec.Command("git", "-C", root, "check-attr", "eol", "--", rel)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git check-attr %s: %v", rel, err)
		}
		if strings.TrimSpace(string(out)) != rel+": eol: lf" {
			t.Errorf("%s must have a working-tree LF attribute, got %q", rel, strings.TrimSpace(string(out)))
		}
	}
}
