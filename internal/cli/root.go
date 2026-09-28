package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/api"
	"github.com/vincentsch/chab-cli/internal/browser"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/commands/authcmd"
	"github.com/vincentsch/chab-cli/internal/commands/credits"
	"github.com/vincentsch/chab-cli/internal/commands/doctor"
	"github.com/vincentsch/chab-cli/internal/commands/management"
	"github.com/vincentsch/chab-cli/internal/commands/mcpcmd"
	operationscmd "github.com/vincentsch/chab-cli/internal/commands/operations"
	"github.com/vincentsch/chab-cli/internal/commands/profile"
	"github.com/vincentsch/chab-cli/internal/commands/projects"
	"github.com/vincentsch/chab-cli/internal/commands/rawapi"
	"github.com/vincentsch/chab-cli/internal/commands/system"
	"github.com/vincentsch/chab-cli/internal/config"
	"github.com/vincentsch/chab-cli/internal/prompt"
)

// globalFlags stores the parsed persistent root flags for one command tree.
// Active commands consume only this narrow root flag set through the shared
// command factory.
type globalFlags struct {
	Profile     string
	Config      string
	AuthFile    string
	BaseURL     string
	APIBaseURL  string
	Locale      string
	JSON        bool
	IncludeMeta bool
	JQ          string
	Template    string
	Plain       bool
	NoColor     bool
	NoANSI      bool
	NoPager     bool
	Debug       bool
	NoPrompt    bool
	Yes         bool
}

const (
	groupAuth      = "authentication"
	groupProjects  = "projects"
	groupConfig    = "configuration"
	groupAPI       = "api"
	groupOps       = "operations"
	groupConnected = "connected-data"
	groupUtilities = "utilities"
)

const rootShort = "Command-line interface for Chab"

const rootLong = `chab is the command-line interface for Chab. It talks to Chab's public
REST API (/v1) using a scoped team API token from browser device login,
manual entry, or CHAB_API_KEY. Browser-issued guest trial credentials can be
used for the backend-advertised free operations without signup.

Quick start:
  chab version --json    # print build metadata for scripts
  chab setup             # set up and verify a copied team or guest key
  chab login             # authorize or replace a stored team API key
  chab whoami --json     # print authenticated team and token data
  chab doctor            # check local setup and API readiness
  chab profile list      # inspect local connection profiles
  chab config path       # print the local config and auth paths
  chab health            # inspect public API health
  chab errors --json     # inspect public API error catalog
  chab search web --help # start a recoverable operation
  chab files list        # inspect stored files
  chab operations schema search.web # inspect embedded operation schema
  chab api get /me --json # call a public API path directly
  chab mcp serve --help  # configure local MCP tools`

// NewRootCommand builds the complete chab command tree, wiring stdout and
// stderr before Cobra's help and completion commands are initialized.
func NewRootCommand(stdout, stderr io.Writer) *cobra.Command {
	return NewRootCommandWith(stdout, stderr, Options{})
}

// NewRootCommandWith builds the complete chab command tree with injected
// process input options.
func NewRootCommandWith(stdout, stderr io.Writer, opts Options) *cobra.Command {
	root, _, err := NewRootCommandWithCatalog(stdout, stderr, opts)
	if err != nil {
		panic(constructionPanicMessage(err))
	}
	return root
}

// NewRootCommandWithCatalog builds the command tree, initializes Cobra-owned
// commands, and validates that the visible tree and catalog agree.
func NewRootCommandWithCatalog(stdout, stderr io.Writer, opts Options) (*cobra.Command, []Entry, error) {
	opts = normalizeOptions(opts)
	flags := &globalFlags{}
	root := &cobra.Command{
		Use:           "chab",
		Short:         rootShort,
		Long:          rootLong,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetIn(opts.Stdin)
	stdoutIsTerminal := opts.StdoutIsTerminal
	if stdoutIsTerminal == nil {
		stdoutWriter := stdout
		stdoutIsTerminal = func() bool {
			return prompt.IsTerminalWriter(stdoutWriter)
		}
	}
	terminalHeight := opts.TerminalHeight
	if terminalHeight == nil {
		stdoutWriter := stdout
		terminalHeight = func() (int, bool) {
			return prompt.TerminalHeightWriter(stdoutWriter)
		}
	}
	factory := &cmdutil.Factory{
		Stdin:            opts.Stdin,
		StdinIsTerminal:  opts.StdinIsTerminal,
		StdoutIsTerminal: stdoutIsTerminal,
		TerminalHeight:   terminalHeight,
		RunPager:         opts.RunPager,
		Sleeper:          opts.Sleeper,
		LookupEnv:        opts.LookupEnv,
		Now:              opts.Now,
		Version:          Version,
		Secrets:          opts.secrets,
		BrowserOpener:    opts.BrowserOpener,
		BootstrapFactory: opts.BootstrapFactory,
	}
	if factory.BrowserOpener == nil {
		factory.BrowserOpener = browser.Open
	}
	if factory.BootstrapFactory == nil {
		factory.BootstrapFactory = func(rt config.Runtime, cmd *cobra.Command) (cmdutil.BootstrapClient, error) {
			bootstrapOpts := api.OptionsForBootstrap(rt, factory.VersionString(), cmdutil.DebugEnabled(cmd), cmd.ErrOrStderr())
			bootstrapOpts.Now = factory.Clock()
			return api.NewBootstrap(bootstrapOpts)
		}
	}

	addRootGroups(root)
	registerGlobalFlags(root, flags)
	registerCommandTree(root, flags, factory)

	// Install custom help before initializing Cobra's defaults. Cobra creates
	// help and completion lazily, but catalog validation needs the final visible
	// tree during construction.
	root.SetHelpCommand(newHelpCommand())
	root.SetHelpCommandGroupID(groupUtilities)
	root.SetCompletionCommandGroupID(groupUtilities)
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	if completion, _, err := root.Find([]string{"completion"}); err == nil && completion != nil {
		// Cobra's generated completion parent treats unknown shell names as a
		// generic argument error. The shell contract needs bare completion to
		// show help, and invalid shell names to render like other local usage
		// errors with the command path attached.
		completion.GroupID = groupUtilities
		completion.Args = cobra.ArbitraryArgs
		completion.RunE = runFamilyHelpOrUnknown
	}

	tagFlagParseErrors(root)
	entries := Catalog()
	installOutputModeGuard(root, entries)
	if err := validateTreeCatalog(root, entries); err != nil {
		return nil, nil, err
	}
	return root, entries, nil
}

func addRootGroups(root *cobra.Command) {
	root.AddGroup(
		&cobra.Group{ID: groupAuth, Title: "Authentication and Identity"},
		&cobra.Group{ID: groupProjects, Title: "Management"},
		&cobra.Group{ID: groupConfig, Title: "Profiles and Configuration"},
		&cobra.Group{ID: groupAPI, Title: "Raw API Access"},
		&cobra.Group{ID: groupOps, Title: "Operations"},
		&cobra.Group{ID: groupConnected, Title: "Files and Connected Data"},
		&cobra.Group{ID: groupUtilities, Title: "Diagnostics and Utilities"},
	)
}

func registerGlobalFlags(root *cobra.Command, flags *globalFlags) {
	persistent := root.PersistentFlags()
	persistent.StringVar(&flags.Profile, "profile", "", "profile to use for this invocation")
	persistent.StringVar(&flags.Config, "config", "", "path to the CLI config file")
	persistent.StringVar(&flags.AuthFile, "auth-file", "", "path to the CLI auth file")
	persistent.StringVar(&flags.BaseURL, "base-url", "", "product web base URL")
	persistent.StringVar(&flags.APIBaseURL, "api-base-url", "", "API base URL (Chab default uses www.chab.ai/v1; custom base URLs use <base-url>/v1)")
	persistent.StringVar(&flags.Locale, "locale", "", "response language sent as Accept-Language (en or de)")
	persistent.BoolVar(&flags.JSON, "json", false, "print stable JSON output where supported")
	persistent.BoolVar(&flags.IncludeMeta, "include-meta", false, "include safe response metadata in machine output where supported")
	persistent.StringVar(&flags.JQ, "jq", "", "filter supported JSON output with a jq expression")
	persistent.StringVar(&flags.Template, "template", "", "render supported JSON output with a Go text/template")
	persistent.BoolVar(&flags.Plain, "plain", false, "print copy-safe plain output where supported")
	persistent.BoolVar(&flags.NoColor, "no-color", false, "disable color in human output where supported")
	persistent.BoolVar(&flags.NoANSI, "no-ansi", false, "disable ANSI terminal controls and paging where supported")
	persistent.BoolVar(&flags.NoPager, "no-pager", false, "disable pager use for long human output where supported")
	persistent.BoolVar(&flags.Debug, "debug", false, "write redacted request details to stderr")
	persistent.BoolVar(&flags.NoPrompt, "no-prompt", false, "fail instead of prompting for interactive input")
	persistent.BoolVar(&flags.Yes, "yes", false, "accept supported confirmations for risky or destructive operations; does not supply missing input")
}

func registerCommandTree(root *cobra.Command, flags *globalFlags, factory *cmdutil.Factory) {
	root.AddCommand(
		newVersionCommand(factory),
		withGroup(authcmd.NewSetupCommand(factory), groupAuth),
		withGroup(authcmd.NewLoginAlias(factory), groupAuth),
		withGroup(authcmd.NewLogoutAlias(factory), groupAuth),
		withGroup(authcmd.NewWhoamiCommand(factory), groupAuth),
		withGroup(authcmd.NewAuthCommand(factory), groupAuth),
		withGroup(profile.NewProfileCommand(factory), groupConfig),
		withGroup(profile.NewConfigCommand(factory), groupConfig),
		newAPICommand(factory),
		newCreditsCommand(factory),
		withGroup(projects.NewProjectCommand(factory), groupProjects),
		withGroup(management.NewUsageCommand(factory), groupProjects),
		withGroup(management.NewBillingCommand(factory), groupProjects),
		withGroup(management.NewTokensCommand(factory), groupProjects),
		withGroup(management.NewWebhooksCommand(factory), groupProjects),
		withGroup(operationscmd.NewOperationsCommand(factory), groupOps),
		withGroup(operationscmd.NewExamplesCommand(factory), groupOps),
		withGroup(operationscmd.NewSearchCommand(factory), groupOps),
		withGroup(operationscmd.NewSEOCommand(factory), groupOps),
		withGroup(operationscmd.NewBusinessCommand(factory), groupOps),
		withGroup(operationscmd.NewContactsCommand(factory), groupOps),
		withGroup(operationscmd.NewScrapeCommand(factory), groupOps),
		withGroup(operationscmd.NewScreenshotsCommand(factory), groupOps),
		withGroup(operationscmd.NewConvertCommand(factory), groupOps),
		withGroup(operationscmd.NewTranslateCommand(factory), groupOps),
		withGroup(operationscmd.NewLLMCommand(factory), groupOps),
		withGroup(operationscmd.NewResearchCommand(factory), groupOps),
		withGroup(operationscmd.NewFilesCommand(factory), groupConnected),
		withGroup(operationscmd.NewMailCommand(factory), groupConnected),
		withGroup(operationscmd.NewDriveCommand(factory), groupConnected),
		withGroup(system.NewHealthCommand(factory), groupUtilities),
		withGroup(system.NewErrorsCommand(factory), groupUtilities),
		withGroup(doctor.NewCommand(factory), groupUtilities),
		withGroup(mcpcmd.NewCommand(factory), groupUtilities),
	)
}

func withGroup(cmd *cobra.Command, group string) *cobra.Command {
	cmd.GroupID = group
	return cmd
}

func newAPICommand(factory *cmdutil.Factory) *cobra.Command {
	cmd := rawapi.NewAPICommand(factory)
	cmd.GroupID = groupAPI
	return cmd
}

func newCreditsCommand(factory *cmdutil.Factory) *cobra.Command {
	cmd := credits.NewCreditsCommand(factory)
	cmd.GroupID = groupProjects
	return cmd
}

// newHelpCommand keeps Cobra's topic-help behavior but owns the failure path.
// That lets invalid topics render as local usage errors with empty stdout while
// still offering dynamic completion for command names.
func newHelpCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "help [command]",
		Short:   summaryFor("help"),
		Long:    "Help provides help for any command in the application.\nType chab help [command] for full details.",
		GroupID: groupUtilities,
		Args:    cobra.ArbitraryArgs,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			target, remaining, err := cmd.Root().Find(args)
			if err != nil || len(remaining) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			if target == nil {
				target = cmd.Root()
			}
			var completions []cobra.Completion
			for _, child := range target.Commands() {
				if child.IsAvailableCommand() || child.Name() == "help" {
					if strings.HasPrefix(child.Name(), toComplete) {
						completions = append(completions, cobra.CompletionWithDesc(child.Name(), child.Short))
					}
				}
			}
			return completions, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			target, remaining, err := cmd.Root().Find(args)
			if err != nil || len(remaining) > 0 {
				name, path := unresolvedTopic(args, target, remaining, cmd.Root())
				return &unknownSubcommandError{Name: name, Path: path}
			}
			if target == nil {
				target = cmd.Root()
			}
			target.InitDefaultHelpFlag()
			return target.Help()
		},
	}
}

// unresolvedTopic reconstructs the command path Cobra had resolved before it
// found an invalid help topic. The renderer uses that path for the usage hint.
func unresolvedTopic(args []string, target *cobra.Command, remaining []string, root *cobra.Command) (string, string) {
	if len(remaining) > 0 {
		path := root.CommandPath()
		if target != nil {
			path = target.CommandPath()
		}
		return remaining[0], path
	}
	if len(args) == 0 {
		return "", root.CommandPath()
	}
	name := args[len(args)-1]
	path := root.CommandPath()
	if len(args) > 1 {
		path = root.CommandPath() + " " + strings.Join(args[:len(args)-1], " ")
	}
	return name, path
}

// runFamilyHelpOrUnknown is shared by reserved command families and the
// completion parent: no args means "show family help"; extra args are unknown
// subcommands rather than feature behavior.
func runFamilyHelpOrUnknown(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	return &unknownSubcommandError{Name: args[0], Path: cmd.CommandPath()}
}

// validateTreeCatalog keeps help, completion, and reserved command registration
// honest at construction time. If a visible command lacks catalog metadata, or
// a catalog row points to no visible command, building the root fails.
func validateTreeCatalog(root *cobra.Command, entries []Entry) error {
	nodes := VisibleCommands(root, false)
	visible := map[string]bool{}
	for _, node := range nodes {
		if visible[node.Path] {
			return fmt.Errorf("duplicate visible command path %q", node.Path)
		}
		visible[node.Path] = true
	}

	catalog := map[string]bool{}
	for _, entry := range entries {
		if catalog[entry.Spec.Path] {
			return fmt.Errorf("duplicate catalog path %q", entry.Spec.Path)
		}
		catalog[entry.Spec.Path] = true
	}
	for _, node := range nodes {
		if !catalog[node.Path] {
			return fmt.Errorf("visible command %q has no catalog entry", node.Path)
		}
	}
	for _, entry := range entries {
		if !visible[entry.Spec.Path] {
			return fmt.Errorf("catalog entry %q does not resolve to a visible command", entry.Spec.Path)
		}
	}
	return nil
}

func constructionPanicMessage(err error) string {
	return "cli: building chab command tree failed: " + err.Error()
}
