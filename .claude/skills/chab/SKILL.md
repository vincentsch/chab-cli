---
name: chab
description: Use the Chab CLI and local MCP server safely for auth, credits, public API diagnostics, raw API, and automation workflows.
---

# Chab CLI

Use this skill when working with a repository or environment that has the `chab`
command available, or when an agent host should inspect Chab state through the
local MCP server.

## Workflow

If `chab` is missing, use verified public release instructions when published;
until then, ask the user to install from a Chab CLI source checkout. After
install, verify the binary before running authenticated
commands:

- `chab version`
- `chab --help`

Start with read-only discovery:

- `chab version`
- `chab --help`
- `chab auth env`
- `chab whoami`
- `chab doctor`
- `chab credits balance`
  - With a guest credential, inspect `free_credits.available`; `spendable_balance` is the team-paid balance and is zero for guests.

Use `chab auth env` to inspect the resolved profile, product URL, API URL, and
locale. It is local-only and never returns a key or contacts the network; it
may inspect whether a selected stored-key profile lacks a saved destination
to preserve its legacy localhost target.
For a fresh profile, the app and API share `https://www.chab.ai` (`/v1` for
API calls); saved explicit destinations remain unchanged.
Use `chab whoami` after login or `CHAB_API_KEY` setup to verify the active team
token or guest principal without exposing the raw credential. Check scopes and project
access before choosing an operation. Use `chab doctor` for combined local and
API readiness checks.

Use normal command output controls only on commands that document support for
them. Prefer `--json` for automation, `--jq` for small projections,
`--template` for stable custom text, and `--plain` for copy-safe tabular output.
Use `--include-meta` only where the command documents response metadata
support.

For a guest trial, inspect `free_credits.available` with `chab credits balance`;
the generic estimate route is not offered to guest credentials. Before paid
work, inspect credits with `chab credits balance`, use
`chab operations estimate <operation-key> --input @request.json` when the API
supports estimates, and prefer `--dry-run` first when the command supports it.
Do not pass `--yes` unless the user already made the destructive or billable
intent clear.

Operation starts and uploads are one logical submission. They create a private
local action record and use one idempotency key. If a submit is interrupted or
uncertain, inspect `chab operations actions list`, then
`chab operations actions show <action-id>`, and recover with
`chab operations resume <action-id> --input @request.json` or the documented
file-specific resume command. Do not start a second logical action while a
matching recoverable action exists.
An existing `prepared` receipt with no persisted response cannot be resumed;
do not silently submit it again, since an earlier denial may have failed to
persist. Explain the uncertainty and get explicit consent before a fresh start.

For byte downloads, always use explicit output paths such as `--output` or the
documented short flag. The CLI refuses overwrites; keep checksums when the
command supports them.

Use `chab api get` for read-only fallback access to public API paths. Use unsafe
raw API methods only when the user asked for that specific operation and the
route is known to be safe for the desired effect. Do not recommend explicit
idempotency keys for secret-producing routes.

## MCP

Configure agent hosts with the command, not credentials:

```sh
codex mcp add chab -- chab mcp serve
claude mcp add --transport stdio chab -- chab mcp serve
```

For a non-default profile, put normal CLI flags before the MCP command:

```sh
chab --profile staging mcp serve
```

The MCP server is local stdio only. It resolves auth, config, profile, and
file-backed auth state at each tool call. Startup, discovery, `tools/list`,
`chab_auth_env`, `chab_health`, and `chab_errors` require no credential.
API-backed operation tools require a stored login from `chab login` or
`CHAB_API_KEY` inherited by the MCP server process. Login, logout, config,
selected-profile, and auth-file changes are reloaded for later calls. Changing `CHAB_API_KEY` in the host environment requires restarting or relaunching the MCP server process.

MCP operation tools are named from public operation IDs, for example
`chab_auth_me`, `chab_credits_get`, `chab_credits_transactions_list`,
`chab_search_web`, `chab_files_create`, and `chab_files_download`. Inputs are
grouped as `path`, `query`, `body`, and `local`. Use `local.confirmation=true`
only when a tool schema requires it and the user confirms the billable,
destructive, security-sensitive, upload or send effect. Use `local.input_path`
for uploads and `local.output_path` for downloads; downloads refuse to
overwrite existing files. Use `local.wait=true` only for tools that return
recoverable actions.

For a browser-issued guest trial credential, use `chab setup` locally or a
user-managed `CHAB_API_KEY` secret. The guest REST bearer is not a hosted MCP
OAuth token. Hosted MCP at the backend `/mcp` URL uses browser OAuth/PKCE,
not the CLI's local stdio/REST key; check live compatibility for availability.
Local MCP discovery follows the backend's guest compatibility list: currently
`chab_search_web` and `chab_contacts_email_verify` are the
free operation starters, with identity, credits, current-operation and own
action-recovery helpers. Do not attempt management, connected-account,
billing, paid-only or operation-history tools under a guest credential.
Promotional credits are one-off; preserve backend challenge, pause, exhaustion,
expiry, revocation and signup-required errors rather than retrying around them.

Recover MCP submissions with `chab_action_list`, `chab_action_show`, and
`chab_action_resume`. These tools expose public receipts without idempotency
keys. A `prepared` receipt is deliberately non-replayable; explain the
uncertainty and ask before any fresh submission. For protected token mutations,
provide approval proof by
`local.approval_proof_path`; never paste proof text into chat or arguments.
One-time secret-producing routes are intentionally not exposed as MCP tools.

Use a dedicated scoped, expiring profile for an agent. Browser login uses
`client_id=chab-cli`, requests only `api:projects:read` by default, and validates
expiry before storing the key. Request additional permissions with `--scope`.
Chab accepts 28 device scopes; `api:billing:write`, `api:tokens:write`, and
`api:webhooks:write` require a separately configured API key and any applicable
browser approval. Spending and project access start at `none`. Approve only
needed permissions, project grants, and a bounded allowance. Older servers may
still restrict device scopes; use their current compatibility guidance.


Pi does not include a built-in MCP client. With this skill installed under
`~/.pi/agent/skills/chab/SKILL.md`, use the `chab` shell commands directly;
do not imply Pi can connect to hosted MCP without an additional extension.

## Safety

Never ask the user to paste, print, store, or save API keys, device codes,
access tokens, token responses, auth-file contents, runtime secret files, or
raw runtime secret values in chat, docs, logs, fixtures, QA evidence, or skill
files. Tell the user to run `chab login` or set `CHAB_API_KEY` in their own
shell or CI secret store.

Never write host MCP config automatically. Show the host config command and let
the user run it in their own environment.

Never store copied logs, screenshots, archives, database dumps, or provider
transcripts in public docs or skill files. Keep durable guidance concise and
redact operational evidence before saving it.
