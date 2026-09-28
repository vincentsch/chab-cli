// Command chab is the reference CLI for the Chab-SaaS public REST API.
package main

import (
	"os"

	"github.com/vincentsch/chab-cli/internal/cli"
)

func main() {
	code, _ := cli.Run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}
