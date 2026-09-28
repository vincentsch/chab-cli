package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/manualdocs"
)

func main() {
	check := flag.Bool("check", false, "check that generated command docs are current")
	flag.Parse()

	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "generate-command-docs: unexpected positional arguments")
		os.Exit(1)
	}

	files, err := generate()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *check {
		result, err := manualdocs.Check(".", files)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if !result.OK() {
			printDrift("missing", result.Missing)
			printDrift("stale", result.Stale)
			printDrift("orphaned", result.Orphaned)
			os.Exit(1)
		}
		return
	}

	if err := manualdocs.Write(".", files); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// generate builds the command tree with inert process dependencies so docs can
// be generated or checked without reading user config, auth files, terminals,
// pagers, or environment variables.
func generate() (map[string][]byte, error) {
	root, entries, err := cli.NewRootCommandWithCatalog(io.Discard, io.Discard, cli.Options{
		LookupEnv:        func(string) (string, bool) { return "", false },
		Stdin:            strings.NewReader(""),
		StdinIsTerminal:  func() bool { return false },
		StdoutIsTerminal: func() bool { return false },
		TerminalHeight:   func() (int, bool) { return 0, false },
	})
	if err != nil {
		return nil, err
	}
	return manualdocs.Generate(root, entries)
}

func printDrift(label string, paths []string) {
	for _, path := range paths {
		fmt.Fprintf(os.Stderr, "%s: %s\n", label, path)
	}
}
