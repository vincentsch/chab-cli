# Chab CLI

Use Chab from your terminal or an agent: search, SEO and business data,
web scraping, screenshots, conversion, translation, LLMs, research, files,
projects, credits, management, connected mail and Drive.

The `chab` binary covers all 98 published API operations. It provides browser
and API-key login, named profiles, JSON output, previews, spending controls,
recoverable submissions, operation polling, and verified file downloads.
`chab mcp serve` exposes 95 local MCP tools over stdio. Credential issuance,
sensitive approval proofs and selected billing/webhook actions remain CLI-only;
see the exact [operation map](docs/operation-map.md).
Browser-issued guest trial credentials work in the CLI and expose only the
backend-advertised free local MCP tools, not the full signed-in catalog.
The historical default profile name `local` targets the Chab app and API
paths on `www.chab.ai` on a fresh install; it does not mean a localhost service. Existing
saved profiles and explicit URL overrides keep their own destinations. A legacy
saved profile with no URL, including auth-only state, stays on localhost until
you explicitly select another destination.

Start with [installation](docs/install.md) and
[getting started](docs/getting-started.md). Use the
[command reference](manual/commands/README.md) to discover commands and flags.
`chab doctor` checks the configured API even without credentials; omit it from
offline-only checks or point both URL overrides at a local mock.
Maintainers can find the shared development conventions in
[framework/README.md](framework/README.md).

## CLI, Skill And MCP

This repository owns the CLI, the optional agent skill, and the local stdio
MCP server. The Chab backend owns the API and hosted MCP. A local MCP host
launches the CLI; a hosted MCP connection uses the backend's separately
configured endpoint and browser OAuth consent. See
[agent access](docs/agent-access.md) for setup and authentication boundaries.

## Current Command Surface

Functional commands:

```bash
chab --help
chab help [command]
chab version
chab version --json
chab version --plain
chab version --jq .version
chab version --template '{{.version}}'
chab completion bash
chab setup
chab setup --plain
chab login
chab login --web
chab login --api-key
chab login --plain
chab whoami --json
chab whoami --plain
chab whoami --jq .team_id
chab whoami --template '{{.token_public_id}}'
chab auth status --json
chab auth status --plain
chab auth env
chab auth env --profile staging --plain
chab auth env --profile staging --json
chab logout
chab logout --plain
chab doctor --json
chab doctor --plain
chab doctor --jq .status
chab doctor --template '{{.status}}'
chab profile list
chab profile list --json
chab profile show staging
chab profile create staging --base-url https://staging.example.test
chab profile use staging
chab profile delete staging --yes
chab config path
chab config list --json
chab config get current_profile
chab config set profiles.staging.locale de
chab credits
chab credits --help
chab credits balance
chab credits balance --json
chab credits balance --jq .balance
chab credits balance --template '{{.balance}}'
chab credits transactions
chab credits transactions --page-size 25 --limit 250
chab credits transactions --json
chab credits transactions --include-meta --jq .meta.pagination.has_more
chab credits transactions --jq '.[].amount'
chab credits transactions --template '{{range .}}{{.id}} {{.amount}}{{"\n"}}{{end}}'
chab api get /me
chab api get /credits --json
chab api get /credits/transactions --all --json --include-meta
chab api get /credits/transactions --raw --jq .data
chab api post /examples/echo --field event=demo --dry-run --json
chab mcp
chab mcp serve --help
```

`chab auth env` is the effective invocation view after flag, environment,
profile-file, normalization, and URL-derivation rules. Persisted `profile show`
and config inspection answer a different question. The report names
`CHAB_API_KEY`, but the secret value still comes from the CI provider's secret
store and is never inspected or printed. Copying both URL rows freezes the API
base derivation until `CHAB_API_BASE_URL` is removed or changed.

Raw paths are confined below the resolved API base; arbitrary URLs, query
markers in paths, and ambiguous bare URI-scheme syntax are rejected locally.
GET supports repeatable query values and shared pagination. Unsafe raw methods
accept optional JSON bodies, preserve validated inline/file/stdin JSON bytes
after trimming surrounding whitespace, use automatic or explicit idempotency on
ordinary unsafe routes, and never prompt for confirmation. Raw POST suppresses
execution for the v1 routes that return one-time plaintext secrets:
`/tokens`, `/webhooks/endpoints`, and
`/webhooks/endpoints/{endpoint_id}/rotate-secret`; use the dedicated
private-output workflow for those operations. Local `--dry-run` returns before config, credentials,
client construction, or HTTP. Profile and config commands operate only on
local `config.yml` and `auth.json`; inspection never prints an API key, and
mutations never contact the API. Credits balance and transactions perform
direct authenticated team-level
`GET /v1/credits` and `GET /v1/credits/transactions` requests.
Cursor pagination is sent to the API; returned rows and time
window ordering remain server-authoritative.

## Opt-In Response Context

`--include-meta` is supported by both `credits` leaves and all four `api`
leaves. It requires
`--json`, `--jq`, or `--template`, and cannot be combined with `--dry-run`.
Without it, successful bytes and transform roots are unchanged. With it, the
ordinary machine value moves under `data` and safe request ids, pagination,
rate limits, retry attempts/waits, and idempotency replay state appear under
`meta`. Decoded raw GET may also expose sanitized, non-pagination envelope
context under `meta.api_meta`; typed commands and `--raw` do not.
Request-local secret and idempotency values are also removed from this
CLI-owned context: matching public rate-limit integers are omitted while raw
text is redacted, matching `api_meta` primitives become redaction-marker
strings, and matching public pagination is omitted. Validated internal
pagination still drives `--all` and `--limit`, so redaction does not truncate
page traversal.

API and protocol failures are not success wrappers. When response headers are
available, their existing human and compact JSON errors include safe
Retry-After and rate-limit context whether or not `--include-meta` was passed.

## Local MCP Server

`chab mcp serve` is a local stdio MCP server for auth environment, public health
and error-catalog checks plus operation-ID tools backed by the public Chab API
contract. It writes protocol frames to stdout, sends redacted diagnostics to
stderr, and exposes no hosted HTTP/SSE transports, resources, prompts, sampling,
roots, or server-initiated requests.

Startup, discovery, `tools/list`, `chab_auth_env`, `chab_health`, and
`chab_errors` require no credential. API-backed tools such as `chab_auth_me`,
`chab_credits_get`, `chab_credits_transactions_list`, and `chab_search_web`
require either `CHAB_API_KEY` inherited by the MCP server process or the
selected profile's auth state written by `chab login`. Confirmation-gated calls
use explicit `local.confirmation=true`; upload/download tools use explicit
local paths and downloads refuse overwrites. Changing `CHAB_API_KEY` in the host environment requires restarting or relaunching the MCP server process.

See [docs/getting-started.md](docs/getting-started.md#connect-a-local-mcp-host)
for host configuration and skill setup.

## Development

Run the shell from a checkout:

```bash
go run ./cmd/chab --help
go run ./cmd/chab version --json
go run ./cmd/chab version --jq .version
go run ./cmd/chab version --plain
go run ./cmd/chab completion bash
```

Run the core checks:

```bash
go test ./...
go test -race ./...
go vet ./...
go fmt ./...
go run ./cmd/chab --help
go run ./cmd/chab version
go run ./cmd/chab version --json
go run ./cmd/chab version --jq .version
go run ./cmd/chab version --plain
go run ./cmd/chab doctor
go run ./cmd/chab profile --help
go run ./cmd/chab profile list --json
go run ./cmd/chab config --help
go run ./cmd/chab config path
go run ./cmd/chab credits --help
go run ./cmd/chab credits balance --help
go run ./cmd/chab credits balance --json
go run ./cmd/chab credits transactions --help
go run ./cmd/chab setup --help
go run ./cmd/chab login --help
go run ./cmd/chab auth login --help
go run ./cmd/chab api --help
go run ./cmd/chab api get --help
go run ./cmd/chab api post /examples/echo --field event=demo --dry-run --json
go run ./cmd/chab mcp --help
go run ./cmd/chab mcp serve --help
go run ./cmd/chab completion bash
go run ./scripts/generate-command-docs --check
go run ./scripts/generate-operation-map --check
go run ./scripts/check-examples
```

Help, version, and completion must not read config files, auth files, API keys,
pager settings, or the network. Profile/config, Credits, and raw API
help are also offline. Profile/config behavior is local-only and never
constructs an API client. Auth/readiness, Credits reads, and real raw
requests use local state and call the API only when their command contract
requires it. Valid raw POST/PATCH/DELETE dry-runs complete before config,
credential, client, or
network resolution. `chab mcp` and `chab mcp serve --help` are offline; do not
run bare `chab mcp serve` outside an MCP stdio host or protocol test harness.

Interactive `chab login` uses first-party browser/device authorization when
the resolved server advertises it. `chab login --api-key` keeps deterministic
manual entry, and non-terminal stdin stays on the manual path without discovery.
`chab login --web --no-prompt` prints the verification URL and user code without
opening a browser.

## Documentation

Cobra help and the generated manual pages are the user-facing command reference
for this shell:

```bash
go run ./cmd/chab help credits transactions
go run ./cmd/chab help version
```

The first-run shell guide is in [docs/getting-started.md](docs/getting-started.md).
The paired-repository strategy is in [docs/agent-access.md](docs/agent-access.md).
The generated command reference is in
[manual/commands/README.md](manual/commands/README.md). Checked automation
workflows are indexed in [examples/README.md](examples/README.md); the checked workflows
run against fresh local mocks or offline previews as part of the repository checker.
Maintainer agent skill support material is in
[.chab-agent-skill/README.md](.chab-agent-skill/README.md).

Command behavior changes must update Cobra help, catalog metadata, help
goldens, generated manual docs, and current-state notes in the same pass.
