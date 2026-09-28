# chab mcp

## Usage

```text
chab mcp [flags]
```

## Description

Run local MCP integrations for agent and editor hosts.

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
tools, not private team-management, connected-account or hosted MCP tools. Changing `CHAB_API_KEY` in the host environment requires restarting or relaunching the MCP server process.

Host config should point at the command, not contain credentials:

```text
codex mcp add chab -- chab mcp serve
claude mcp add --transport stdio chab -- chab mcp serve
```

Use normal chab flags for non-default profiles, for example:

```text
chab --profile staging mcp serve
```

Related commands:
- [chab auth env](chab-auth-env.md)
- [chab login](chab-login.md)
- [chab doctor](chab-doctor.md)

## Subcommands

- [chab mcp serve](chab-mcp-serve.md) - Serve Chab tools over MCP stdio

## Examples

```text
  chab mcp --help
  chab mcp serve --help
  codex mcp add chab -- chab mcp serve
  claude mcp add --transport stdio chab -- chab mcp serve
```

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.

Startup, discovery, `tools/list`, `chab_auth_env`, `chab_health`, and `chab_errors` do not require credentials. API-backed tools such as `chab_auth_me`, `chab_credits_get`, `chab_credits_transactions_list`, and `chab_search_web` require a stored login or `CHAB_API_KEY` inherited by the MCP server process. Confirmation-gated tools require `local.confirmation=true` and recoverable action tools can be inspected with `chab_action_list`, `chab_action_show`, and `chab_action_resume`. Changing `CHAB_API_KEY` in the host environment requires restarting or relaunching the MCP server process.
