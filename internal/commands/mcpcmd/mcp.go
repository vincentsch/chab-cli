package mcpcmd

import (
	"github.com/spf13/cobra"
	"github.com/vincentsch/chab-cli/internal/cmdutil"
	"github.com/vincentsch/chab-cli/internal/mcpserver"
)

const mcpRestartSentence = "Changing `CHAB_API_KEY` in the host environment requires restarting or relaunching the MCP server process."

// NewCommand builds chab mcp.
func NewCommand(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run local MCP integrations",
		Long: `Run local MCP integrations for agent and editor hosts.

The MCP server is local stdio only. It exposes local auth-environment,
health and error-catalog tools plus operation-ID tools backed by the public
Chab API contract. It does not host HTTP, SSE, prompts, resources, sampling,
roots, or server-initiated requests.

Startup, discovery, tools/list, chab_auth_env, chab_health, and chab_errors
require no credential. API-backed tools such as chab_auth_me,
chab_credits_get, chab_credits_transactions_list and chab_search_web require
either CHAB_API_KEY inherited by the MCP server process or the selected
profile's auth state written by chab login. Confirmation-gated tools require
local.confirmation=true and recoverable action tools can be inspected with
chab_action_list, chab_action_show and chab_action_resume. A browser-issued
guest trial credential exposes only the backend-advertised free local MCP
tools, not team, paid-only or hosted MCP tools. ` + mcpRestartSentence + `

Host config should point at the command, not contain credentials:
  codex mcp add chab -- chab mcp serve
  claude mcp add --transport stdio chab -- chab mcp serve

Use normal chab flags for non-default profiles, for example:
  chab --profile staging mcp serve

Related commands:
  chab auth env
  chab login
  chab doctor`,
		Example: `  chab mcp --help
  chab mcp serve --help
  codex mcp add chab -- chab mcp serve
  claude mcp add --transport stdio chab -- chab mcp serve`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(NewServeCommand(f))
	return cmd
}

// NewServeCommand builds chab mcp serve.
func NewServeCommand(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Serve Chab tools over MCP stdio",
		Long: `Serve Chab tools over local MCP stdio.

The command reserves stdout for newline-delimited MCP JSON-RPC frames. Redacted
diagnostics, including --debug request logs, go to stderr. Run this command through an MCP host or protocol test harness; direct terminal execution waits for protocol frames.

Startup, discovery, tools/list, chab_auth_env, chab_health, and chab_errors
require no credential. API-backed operation tools are named from operation IDs,
for example chab_auth_me, chab_credits_get, chab_search_web,
chab_files_create and chab_files_download. Those tools require either
CHAB_API_KEY inherited by the MCP server process or the selected profile's auth
state written by chab login. Login, logout, config, selected-profile, and
auth-file changes are reloaded for later calls. Guest trial credentials are
limited to backend-advertised free operation tools and safe helpers; they are
not hosted MCP OAuth tokens. ` + mcpRestartSentence + `

Tool inputs use path, query, body and local groups. Confirmation-gated tools
require local.confirmation=true. Upload and download tools require local file
paths and downloads refuse overwrites. Recoverable action receipts are
available through chab_action_list, chab_action_show and chab_action_resume.

The inherited --json flag is ignored because normal stdout is the protocol
stream. Transform flags such as --jq, --template, --plain, and --include-meta
are rejected before the server starts.

Related commands:
  chab mcp
  chab auth env
  chab whoami`,
		Example: `  codex mcp add chab -- chab mcp serve
  claude mcp add --transport stdio chab -- chab mcp serve
  chab mcp serve   # protocol testing only; hosts launch this
  chab --profile staging mcp serve`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return mcpserver.Serve(cmd.Context(), mcpserver.Options{
				Factory: f,
				Command: cmd,
				Version: f.VersionString(),
				Stdin:   cmd.InOrStdin(),
				Stdout:  cmd.OutOrStdout(),
				Stderr:  cmd.ErrOrStderr(),
			})
		},
	}
}
