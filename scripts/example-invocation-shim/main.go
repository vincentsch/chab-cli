// Command example-invocation-shim records value-blind command usage before
// forwarding an invocation to the real chab binary.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/pflag"
	"github.com/vincentsch/chab-cli/internal/cli"
)

const shimFailureExit = 125

// invocationTraceRecord captures only command structure. Arguments and flag
// values are deliberately excluded because they may contain protected data.
type invocationTraceRecord struct {
	Path  []string `json:"path"`
	Flags []string `json:"flags"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.LookupEnv))
}

// run records the resolved invocation before forwarding it unchanged to the
// real binary. Shim-owned failures use a dedicated exit code; child exit codes
// pass through exactly.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	realBinary, ok := nonblankEnv(lookupEnv, "CHAB_EXAMPLE_REAL_BIN")
	if !ok {
		writeShimError(stderr, "configuration is unavailable")
		return shimFailureExit
	}
	tracePath, ok := nonblankEnv(lookupEnv, "CHAB_EXAMPLE_TRACE_PATH")
	if !ok {
		writeShimError(stderr, "configuration is unavailable")
		return shimFailureExit
	}

	record, err := resolveInvocation(args)
	if err != nil {
		writeShimError(stderr, "command resolution failed")
		return shimFailureExit
	}
	if err := appendTrace(tracePath, record); err != nil {
		writeShimError(stderr, "trace write failed")
		return shimFailureExit
	}

	command := exec.Command(realBinary, args...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if code := exitErr.ExitCode(); code >= 0 {
				return code
			}
			writeShimError(stderr, "child terminated abnormally")
			return shimFailureExit
		}
		writeShimError(stderr, "child start failed")
		return shimFailureExit
	}
	return 0
}

func nonblankEnv(lookupEnv func(string) (string, bool), name string) (string, bool) {
	value, ok := lookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

// resolveInvocation uses the real Cobra tree to identify the selected command
// and only the flags explicitly visited by this invocation.
func resolveInvocation(args []string) (invocationTraceRecord, error) {
	root := cli.NewRootCommand(io.Discard, io.Discard)
	command, remaining, err := root.Find(args)
	if err != nil || command == nil || command == root {
		return invocationTraceRecord{}, fmt.Errorf("unresolved invocation")
	}
	if err := command.ParseFlags(remaining); err != nil {
		return invocationTraceRecord{}, fmt.Errorf("invalid invocation flags")
	}

	path := strings.TrimPrefix(command.CommandPath(), root.Name()+" ")
	elements := strings.Fields(path)
	if len(elements) == 0 || path == command.CommandPath() {
		return invocationTraceRecord{}, fmt.Errorf("invalid command path")
	}
	seen := make(map[string]struct{})
	visit := func(flag *pflag.Flag) {
		seen[flag.Name] = struct{}{}
	}
	// command.Flags currently records parsed local and persistent flags.
	// InheritedFlags is also visited as a defensive union; seen removes any
	// duplicates without changing the recorded shape.
	command.Flags().Visit(visit)
	command.InheritedFlags().Visit(visit)
	flags := make([]string, 0, len(seen))
	for name := range seen {
		flags = append(flags, name)
	}
	sort.Strings(flags)
	return invocationTraceRecord{Path: elements, Flags: flags}, nil
}

// appendTrace writes one compact record before the child starts and restores
// private permissions even when an existing trace file had broader mode bits.
func appendTrace(path string, record invocationTraceRecord) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	encoded, err := json.Marshal(record)
	if err == nil {
		encoded = append(encoded, '\n')
		_, err = file.Write(encoded)
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func writeShimError(stderr io.Writer, message string) {
	_, _ = fmt.Fprintf(stderr, "chab example shim: %s\n", message)
}
