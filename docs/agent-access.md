# CLI, Skill And MCP

Choose the interface that fits your workflow. The CLI works on its own;
the skill and MCP transports are optional ways to use it with an agent.

| Option | What you use | Credentials | Availability |
| --- | --- | --- | --- |
| CLI | Installed product binary, currently `chab` | Team API key, or browser-issued guest trial credential for supported free operations | Implemented |
| Skill | Product instructions in `SKILL.md` | Uses the CLI; has no independent authority | Standalone Codex/Claude entrypoints implemented |
| Local MCP | Host launches `chab mcp serve` | Selected CLI profile or inherited API-key variable; guest credentials are limited to advertised free tools | Implemented; local utilities and operation-ID tools |
| Hosted MCP | Host connects to product HTTPS `/mcp` | Separate browser OAuth consent and host-held credentials | Separate backend configuration and rollout |

The [operation map](operation-map.md) lists CLI and local MCP coverage,
including deliberate exclusions. Hosted tool availability is configured separately.
The default profile is historically named `local` but on a fresh setup targets
Chab's app/API hosts, not localhost; local MCP uses that same selected profile.
Existing saved destinations are not migrated, and a legacy saved profile with
an omitted URL (or auth-only state) keeps localhost until explicitly changed.

## CLI And Skill

Follow [installation](install.md), then [login](getting-started.md#authorize-the-cli).
The CLI works without an agent. Add the optional
[skill](getting-started.md#install-agent-skills) when an agent should follow its
discovery, output and safe-write workflow. Install the binary separately.

There is one logical product skill with two host-specific entrypoints.
Installed users copy only `SKILL.md`; maintainer support files are not runtime
dependencies. Skills contain no credentials and cannot approve access.

## Local MCP

Follow [local MCP setup](getting-started.md#connect-a-local-mcp-host). The host
launches the installed binary; the MCP server reuses services and the API client
in-process, rather than shelling out to individual commands. It needs no
hosted MCP deployment.

Local utilities cover auth environment, public health, the error catalog and
private action receipts. API-backed operation tools are named from public
operation IDs, such as `chab_auth_me`, `chab_credits_get`,
`chab_credits_transactions_list`, `chab_search_web`, `chab_files_create` and
`chab_files_download`. Listing a tool does not grant authority.
`chab_health` includes `free_mode.state` and each family's
`covered_operation_keys`; a healthy family only proves the listed operations
are covered, not that all operations in that family are startable.

A guest can issue a short-lived trial credential in the browser, then enter it
locally with `chab setup` or `chab login --api-key`. Do not paste the bearer into
agent chat or host configuration. For a guest profile, local MCP consults the
backend compatibility document and currently lists only `chab_search_web` and
`chab_contacts_email_verify` as operation starters, plus safe identity, balance,
current-operation status/result/artifact/download/cancel, and own action
receipts. It denies team management, billing, connected accounts, paid-only
operations and operation-history listing before sending those calls. The
backend still decides balance, risk challenges, expiry, revocation and claim.
Guest action receipts are tied to the principal ID from `/v1/me`; a rotated
credential cannot resume another principal's action.
Definitive API denials on a first submission are terminal local actions. If a
submission is already `unknown`, a later error without an explicit `released`
idempotency outcome does not prove the first attempt failed—even a saved
(`settled`) error can coexist with earlier committed work. The CLI and local
MCP keep that receipt `unknown` and its original key for same-key recovery or
operator reconciliation; do not create a fresh key to bypass it. Only an
explicit `released` outcome closes an unknown replay as denied. An explicit
resume makes one physical mutation attempt; a returned 429 with `Retry-After`
does not trigger a hidden second submission inside the CLI or local MCP. If the
same-key resume keeps returning unmarked or `settled` errors, stop and ask for
operator reconciliation; do not loop indefinitely or silently use a new key.

Startup, discovery, `chab_auth_env`, `chab_health`, and `chab_errors` need no
credential. API calls use the selected profile's stored key or the environment
inherited by the server. Confirmation-gated tools require
`local.confirmation=true`; upload and download tools use explicit local paths,
and downloads refuse overwrites. Recoverable submissions can be inspected with
`chab_action_list`, `chab_action_show` and `chab_action_resume`. A receipt
still in `prepared` state has no persisted response and cannot be replayed;
if a local write failed after denial, replay could start new work. Explain
that ambiguity and get explicit consent before creating a fresh submission.
Use explicit profiles and distinct host entry names for different products or
environments. Never put keys into host config or skill files.

Use a dedicated scoped, expiring profile for an agent. Browser login uses
`client_id=chab-cli`, requests `api:projects:read` by default, and validates
expiry before storing the key. Request additional permissions with `--scope`.
Chab supports 28 device scopes; `api:billing:write`, `api:tokens:write`, and
`api:webhooks:write` require a separately configured API key. Approval starts
with spending and project access at `none`. Enable only the required scopes,
project grants, and bounded allowance. See [login guidance](getting-started.md#authorize-the-cli).


## Hosted MCP

Hosted MCP lets a host connect to a configured backend endpoint and complete
browser OAuth consent. It requires neither this binary nor the skill. Its
credentials, enabled tools, and deployment are managed by the Chab backend.
Local MCP availability does not mean the hosted endpoint is enabled.
The guest REST bearer used by local MCP is not a hosted OAuth token. The
backend `/mcp` endpoint uses browser OAuth/PKCE, including a guest OAuth path
when `guest_oauth_enabled` is advertised in live compatibility. This CLI does
not automate that OAuth flow or prove an endpoint is deployed.

Follow the backend's current hosted MCP setup and consent guidance for the
actual environment you use. Approval is checked against the current OAuth
principal and backend policy on each call; removing a local profile does not
revoke a hosted connection.

Use separate entries such as `product-local` and `product-hosted` if you use
both transports. Their credentials and tool catalogs are independent.

## Removal And Updates

Removing a skill only removes guidance. Removing a local MCP host entry does
not remove its stored CLI key. CLI logout removes the profile credential but
does not revoke the server key or an environment credential; revoke the key
in the SaaS to invalidate its authority.

Removing a hosted MCP entry does not revoke the server-side connection.
Revoke it in the SaaS's MCP Connections page. That does not revoke independent
CLI keys. Hosted MCP does not read CLI files or accept a team API key as an
OAuth shortcut.

Copied skills and installed binaries do not update when source changes. Follow
the product's versioned installation/update instructions and restart a local
MCP process after binary or inherited-environment changes. Developers own
[distribution and compatibility](../framework/distribution-and-maintenance.md).
