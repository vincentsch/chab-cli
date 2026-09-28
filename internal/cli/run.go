// Package cli builds the chab root command, global flags, command catalog, and
// execution entry point.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/prompt"
	"github.com/vincentsch/chab-cli/internal/redact"
)

// Options injects process inputs the shell cannot derive from output writers.
type Options struct {
	LookupEnv        func(string) (string, bool)
	Stdin            io.Reader
	StdinIsTerminal  func() bool
	StdoutIsTerminal func() bool
	TerminalHeight   func() (int, bool)
	RunPager         cmdutil.PagerRunner
	Sleeper          api.Sleeper
	BrowserOpener    cmdutil.BrowserOpener
	BootstrapFactory cmdutil.BootstrapClientFactory
	Now              func() time.Time
	Context          context.Context
	secrets          cmdutil.SecretRegistry
}

// Run executes the chab root command with args, writing user output to stdout
// and errors to stderr. It returns the classified exit code and the error the
// command tree returned, already printed to stderr.
func Run(args []string, stdout, stderr io.Writer) (int, error) {
	return RunWith(args, stdout, stderr, Options{})
}

// RunWith executes the chab root command with injected process inputs.
func RunWith(args []string, stdout, stderr io.Writer, opts Options) (int, error) {
	opts = normalizeOptions(opts)
	registerArgumentSecrets(args, opts.secrets)
	root := NewRootCommandWith(stdout, stderr, opts)
	if args == nil {
		args = []string{}
	}
	root.SetArgs(args)
	ctx, stop := signal.NotifyContext(opts.Context, os.Interrupt, syscall.SIGTERM)
	defer stop()
	root.SetContext(ctx)

	err := root.Execute()
	if err == nil {
		return ExitSuccess, nil
	}
	err = (&cmdutil.Factory{Secrets: opts.secrets}).RedactError(err)
	if stderr != nil {
		// The error itself has already been exact-redacted. API errors also
		// redact their structured fields before rendering, so a second raw-text
		// pass here would only risk rewriting JSON syntax and field names.
		renderError(stderr, err, cmdutil.MachineOutputEnabled(root))
	}
	return ExitCodeFor(err), err
}

// registerArgumentSecrets runs before Cobra parses flags so parse and output-
// preflight errors cannot echo an explicit idempotency key. The command still
// validates the key and decides whether it is usable later in its normal flow.
func registerArgumentSecrets(args []string, secrets cmdutil.SecretRegistry) {
	if secrets == nil {
		return
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			return
		}
		if arg == "--idempotency-key" {
			if index+1 < len(args) {
				secrets.RegisterSecret(args[index+1])
				index++
			}
			continue
		}
		if strings.HasPrefix(arg, "--idempotency-key=") {
			secrets.RegisterSecret(strings.TrimPrefix(arg, "--idempotency-key="))
		}
	}
}

func normalizeOptions(opts Options) Options {
	if opts.LookupEnv == nil {
		opts.LookupEnv = os.LookupEnv
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.StdinIsTerminal == nil {
		stdin := opts.Stdin
		opts.StdinIsTerminal = func() bool {
			return prompt.IsTerminalReader(stdin)
		}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	if opts.RunPager == nil {
		opts.RunPager = cmdutil.DefaultPagerRunner
	}
	if opts.secrets == nil {
		opts.secrets = redact.NewRegistry()
	}
	return opts
}

func renderError(stderr io.Writer, err error, jsonMode bool) {
	if stderr == nil {
		return
	}
	if cmdutil.RenderAPIError(stderr, err, jsonMode) {
		return
	}
	fmt.Fprintf(stderr, "Error: %s\n", err)

	// Only local usage-style errors get an extra help hint. Other command
	// errors can carry their own user guidance without the runner appending
	// unrelated Cobra text.
	var parseErr *flagParseError
	if errors.As(err, &parseErr) {
		fmt.Fprintf(stderr, "Run %q for usage.\n", parseErr.path+" --help")
	}

	var unknownErr *unknownSubcommandError
	if errors.As(err, &unknownErr) && unknownErr.Path != "" {
		fmt.Fprintf(stderr, "Run %q for usage.\n", unknownErr.Path+" --help")
	}

	var cmdutilUnknown *cmdutil.UnknownSubcommandError
	if errors.As(err, &cmdutilUnknown) && cmdutilUnknown.Path != "" {
		fmt.Fprintf(stderr, "Run %q for usage.\n", cmdutilUnknown.Path+" --help")
	}
}

// tagFlagParseErrors wraps Cobra flag parser failures on every command so the
// renderer can name the exact command whose flags failed.
func tagFlagParseErrors(root *cobra.Command) {
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
			return &flagParseError{path: c.CommandPath(), err: err}
		})
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
}
