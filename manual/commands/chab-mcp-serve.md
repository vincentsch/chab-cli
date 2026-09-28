# chab mcp serve

## Usage

```text
chab mcp serve [flags]
```

## Description

Serve Chab tools over local MCP stdio.

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
not hosted MCP OAuth tokens. Changing `CHAB_API_KEY` in the host environment requires restarting or relaunching the MCP server process.

Tool inputs use path, query, body and local groups. Confirmation-gated tools
require local.confirmation=true. Upload and download tools require local file
paths and downloads refuse overwrites. Recoverable action receipts are
available through chab_action_list, chab_action_show and chab_action_resume.

The inherited --json flag is ignored because normal stdout is the protocol
stream. Transform flags such as --jq, --template, --plain, and --include-meta
are rejected before the server starts.

Related commands:
- [chab mcp](chab-mcp.md)
- [chab auth env](chab-auth-env.md)
- [chab whoami](chab-whoami.md)

## Examples

```text
  codex mcp add chab -- chab mcp serve
  claude mcp add --transport stdio chab -- chab mcp serve
  chab mcp serve   # protocol testing only; hosts launch this
  chab --profile staging mcp serve
```

## Output modes

MCP stdio

## Authentication

Runs without requiring a credential.

Startup, discovery, `tools/list`, `chab_auth_env`, `chab_health`, and `chab_errors` do not require credentials. API-backed tools such as `chab_auth_me`, `chab_credits_get`, `chab_credits_transactions_list`, and `chab_search_web` require a stored login or `CHAB_API_KEY` inherited by the MCP server process. Confirmation-gated tools require `local.confirmation=true` and recoverable action tools can be inspected with `chab_action_list`, `chab_action_show`, and `chab_action_resume`. Changing `CHAB_API_KEY` in the host environment requires restarting or relaunching the MCP server process.
