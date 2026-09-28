package examplecheck

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadTraceStrictRecordsAndMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.ndjson")
	content := "{\"path\":[\"api\",\"get\"],\"flags\":[\"json\"]}\n\n" +
		"{\"path\":[\"api\",\"get\"],\"flags\":[\"include-meta\",\"json\"]}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	records, raw, err := loadTrace(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || string(raw) != content {
		t.Fatalf("records=%#v raw=%q", records, raw)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadTrace(path); err == nil || !strings.Contains(err.Error(), "want 0600") {
			t.Fatalf("mode error = %v", err)
		}
	}
}

func TestLoadTraceRejectsMalformedRecords(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "unknown field", line: `{"path":["api","get"],"flags":[],"value":"secret"}`, want: "unknown field"},
		{name: "trailing", line: `{"path":["api","get"],"flags":[]} {}`, want: "trailing data"},
		{name: "missing flags", line: `{"path":["api","get"]}`, want: "non-null path and flags"},
		{name: "null flags", line: `{"path":["api","get"],"flags":null}`, want: "non-null path and flags"},
		{name: "empty path", line: `{"path":[],"flags":[]}`, want: "path must not be empty"},
		{name: "blank element", line: `{"path":["api"," "],"flags":[]}`, want: "path[1] must not be blank"},
		{name: "whitespace element", line: `{"path":["api get"],"flags":[]}`, want: "path[0] must not contain whitespace"},
		{name: "blank flag", line: `{"path":["api","get"],"flags":[""]}`, want: "flags[0] must not be blank"},
		{name: "unsorted flags", line: `{"path":["api","get"],"flags":["json","include-meta"]}`, want: "strictly sorted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "trace.ndjson")
			if err := os.WriteFile(path, []byte(tt.line+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := loadTrace(path); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("loadTrace error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCompareTraceKeepsPathElementBoundariesDistinct(t *testing.T) {
	example := Example{Commands: []CommandUse{
		{Path: []string{"api get"}},
	}}
	observed := []traceRecord{
		{Path: []string{"api", "get"}},
	}
	got := joinErrors(compareTrace(example, observed))
	for _, want := range []string{
		`observed command "api get" is not declared`,
		`declared command "api get" was not observed`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("errors missing %q:\n%s", want, got)
		}
	}
}

func TestCompareTraceReportsBothDirectionsAndUnionsFlags(t *testing.T) {
	example := Example{Commands: []CommandUse{
		{Path: []string{"api", "get"}, Flags: []string{"include-meta", "json"}},
		{Path: []string{"credits", "balance"}, Flags: []string{"json"}},
	}}
	observed := []traceRecord{
		{Path: []string{"api", "get"}, Flags: []string{"json"}},
		{Path: []string{"api", "get"}, Flags: []string{"plain"}},
		{Path: []string{"api", "get"}, Flags: []string{"json"}},
	}
	got := joinErrors(compareTrace(example, observed))
	for _, want := range []string{
		`observed flag "--plain" for command "api get" is not declared`,
		`declared command "credits balance" was not observed`,
		`declared flag "--include-meta" for command "api get" was not observed`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("errors missing %q:\n%s", want, got)
		}
	}
}
