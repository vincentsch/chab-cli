package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/vincentsch/chab-cli/internal/examplecheck"
)

func main() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintln(out, "Validate and run the executable automation examples from the repository root.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "The checker builds chab, examples/mockapi, and scripts/example-invocation-shim,")
		fmt.Fprintln(out, "then validates examples/check-examples.json")
		fmt.Fprintln(out, "against the Cobra command tree and catalog, and runs every non-live example")
		fmt.Fprintln(out, "against a fresh local mock. examples/live is never discovered or executed.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Flags:")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "check-examples: unexpected positional arguments")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := examplecheck.Run(examplecheck.RunOptions{RepoRoot: ".", Stderr: os.Stderr, Context: ctx}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
