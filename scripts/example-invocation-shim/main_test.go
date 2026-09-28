package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveInvocationRecordsCanonicalPathAndVisitedFlags(t *testing.T) {
	record, err := resolveInvocation([]string{
		"api", "post", "/projects",
		"--field", "name=value-that-must-not-be-recorded",
		"--json",
		"--no-prompt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(record.Path, " ") != "api post" {
		t.Fatalf("path = %#v", record.Path)
	}
	if got, want := strings.Join(record.Flags, ","), "field,json,no-prompt"; got != want {
		t.Fatalf("flags = %q, want %q", got, want)
	}
	encoded := traceJSON(t, record)
	if strings.Contains(encoded, "value-that-must-not-be-recorded") || strings.Contains(encoded, "/projects") {
		t.Fatalf("record contains invocation values: %s", encoded)
	}
}

func TestAppendTraceIsCompactAppendOnlyAndMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.ndjson")
	first := invocationTraceRecord{Path: []string{"project", "list"}, Flags: []string{"json"}}
	second := invocationTraceRecord{Path: []string{"api", "get"}, Flags: []string{"include-meta", "jq"}}
	if err := appendTrace(path, first); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := appendTrace(path, second); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"path\":[\"project\",\"list\"],\"flags\":[\"json\"]}\n" +
		"{\"path\":[\"api\",\"get\"],\"flags\":[\"include-meta\",\"jq\"]}\n"
	if string(data) != want {
		t.Fatalf("trace = %q, want %q", data, want)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %04o, want 0600", info.Mode().Perm())
		}
	}
}

func TestRunForwardsStdioAndExactExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX forwarding fixture")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real-chab")
	script := "#!/bin/sh\nread line\nprintf 'out:%s\\n' \"$line\"\nprintf 'env:%s\\n' \"$SHIM_FORWARD_MARKER\"\nprintf 'cwd:%s\\n' \"$PWD\"\nprintf 'err:%s\\n' \"$1\" >&2\nexit 5\n"
	if err := os.WriteFile(real, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHIM_FORWARD_MARKER", "forwarded")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(dir, "trace.ndjson")
	env := map[string]string{
		"CHAB_EXAMPLE_REAL_BIN":   real,
		"CHAB_EXAMPLE_TRACE_PATH": trace,
	}
	lookup := func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"api", "get", "/missing", "--json"}, strings.NewReader("input\n"), &stdout, &stderr, lookup)
	if code != 5 {
		t.Fatalf("exit = %d, want 5; stderr=%q", code, stderr.String())
	}
	wantStdout := "out:input\nenv:forwarded\ncwd:" + cwd + "\n"
	if stdout.String() != wantStdout || stderr.String() != "err:api\n" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunUsesFixedValueBlindFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	secretArg := "value-that-must-stay-blind"
	code := run([]string{"api", "get", secretArg}, strings.NewReader(""), &stdout, &stderr, func(string) (string, bool) {
		return "", false
	})
	if code != shimFailureExit || code == 5 {
		t.Fatalf("exit = %d, want fixed %d", code, shimFailureExit)
	}
	if strings.Contains(stderr.String(), secretArg) || stderr.String() != "chab example shim: configuration is unavailable\n" {
		t.Fatalf("stderr is not generic: %q", stderr.String())
	}
}

func TestRunResolutionAndChildStartErrorsRemainValueBlind(t *testing.T) {
	dir := t.TempDir()
	trace := filepath.Join(dir, "trace.ndjson")
	secretArg := "argument-value-that-must-stay-blind"
	tests := []struct {
		name string
		args []string
		real string
		want string
	}{
		{
			name: "parse",
			args: []string{"api", "get", "/projects", "--not-a-real-flag", secretArg},
			real: filepath.Join(dir, "unused"),
			want: "chab example shim: command resolution failed\n",
		},
		{
			name: "start",
			args: []string{"api", "get", secretArg, "--json"},
			real: filepath.Join(dir, "missing-"+secretArg),
			want: "chab example shim: child start failed\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{
				"CHAB_EXAMPLE_REAL_BIN":   tt.real,
				"CHAB_EXAMPLE_TRACE_PATH": trace,
			}
			var stderr bytes.Buffer
			code := run(tt.args, strings.NewReader(""), io.Discard, &stderr, func(name string) (string, bool) {
				value, ok := env[name]
				return value, ok
			})
			if code != shimFailureExit || stderr.String() != tt.want {
				t.Fatalf("exit=%d stderr=%q", code, stderr.String())
			}
			if strings.Contains(stderr.String(), secretArg) || strings.Contains(stderr.String(), tt.real) {
				t.Fatalf("stderr contains invocation value: %q", stderr.String())
			}
		})
	}
}

func TestRunTraceAndAbnormalTerminationErrorsRemainValueBlind(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX abnormal-termination fixture")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real-chab")
	if err := os.WriteFile(real, []byte("#!/bin/sh\nkill -TERM $$\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	secretArg := "argument-value-that-must-stay-blind"
	tests := []struct {
		name  string
		trace string
		want  string
	}{
		{
			name:  "trace",
			trace: dir,
			want:  "chab example shim: trace write failed\n",
		},
		{
			name:  "abnormal termination",
			trace: filepath.Join(dir, "trace.ndjson"),
			want:  "chab example shim: child terminated abnormally\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{
				"CHAB_EXAMPLE_REAL_BIN":   real,
				"CHAB_EXAMPLE_TRACE_PATH": tt.trace,
			}
			var stderr bytes.Buffer
			code := run([]string{"api", "get", secretArg, "--json"}, strings.NewReader(""), io.Discard, &stderr, func(name string) (string, bool) {
				value, ok := env[name]
				return value, ok
			})
			if code != shimFailureExit || stderr.String() != tt.want {
				t.Fatalf("exit=%d stderr=%q", code, stderr.String())
			}
			if strings.Contains(stderr.String(), secretArg) {
				t.Fatalf("stderr contains invocation value: %q", stderr.String())
			}
		})
	}
}

func traceJSON(t *testing.T, record invocationTraceRecord) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trace")
	if err := appendTrace(path, record); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
