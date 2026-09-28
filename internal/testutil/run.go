package testutil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cli"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/config"
)

// Result captures one in-process chab invocation.
type Result struct {
	Stdout   string
	Stderr   string
	Err      error
	ExitCode int
}

// Options injects per-invocation process inputs for command tests.
type Options struct {
	Stdin            io.Reader
	LookupEnv        func(string) (string, bool)
	StdinIsTerminal  func() bool
	StdoutIsTerminal func() bool
	TerminalHeight   func() (int, bool)
	RunPager         cmdutil.PagerRunner
	Sleeper          api.Sleeper
	BrowserOpener    cmdutil.BrowserOpener
	BootstrapFactory cmdutil.BootstrapClientFactory
	Now              func() time.Time
	Context          context.Context
}

// RunCommand executes the chab root command in-process with args and captures
// stdout, stderr, the returned error, and the classified exit code. It never
// spawns a subprocess.
func RunCommand(t *testing.T, args ...string) Result {
	return RunCommandWith(t, Options{}, args...)
}

// RunCommandWith executes the chab root command in-process with injected
// process inputs.
func RunCommandWith(t *testing.T, opts Options, args ...string) Result {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code, err := cli.RunWith(args, &stdout, &stderr, cliOptions(opts))
	return Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Err:      err,
		ExitCode: code,
	}
}

func cliOptions(opts Options) cli.Options {
	stdin := opts.Stdin
	if stdin == nil {
		stdin = bytes.NewReader(nil)
	}
	opener := opts.BrowserOpener
	if opener == nil {
		opener = func(context.Context, string) error {
			return fmt.Errorf("test browser opener was not configured")
		}
	}
	bootstrapFactory := opts.BootstrapFactory
	if bootstrapFactory == nil {
		bootstrapFactory = func(config.Runtime, *cobra.Command) (cmdutil.BootstrapClient, error) {
			return nil, fmt.Errorf("test bootstrap client was not configured")
		}
	}
	return cli.Options{
		Stdin:            stdin,
		LookupEnv:        opts.LookupEnv,
		StdinIsTerminal:  opts.StdinIsTerminal,
		StdoutIsTerminal: opts.StdoutIsTerminal,
		TerminalHeight:   opts.TerminalHeight,
		RunPager:         opts.RunPager,
		Sleeper:          opts.Sleeper,
		BrowserOpener:    opener,
		BootstrapFactory: bootstrapFactory,
		Now:              opts.Now,
		Context:          opts.Context,
	}
}
