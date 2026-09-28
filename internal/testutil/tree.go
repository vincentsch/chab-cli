package testutil

import (
	"io"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cli"
)

// BuildTree constructs a command tree plus its validated per-tree catalog
// without executing it and without panicking on construction failure.
func BuildTree(stdout, stderr io.Writer, opts Options) (*cobra.Command, []cli.Entry, error) {
	return cli.NewRootCommandWithCatalog(stdout, stderr, cliOptions(opts))
}
