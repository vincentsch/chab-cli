# Getting Started

Native commands cover Chab search, SEO, scraping, screenshots, conversion,
translation, LLMs, research, files, operations, management and connected data;
see the [operation map](operation-map.md) for the complete command list.

This guide starts with the common `chab` setup and authentication paths: offline help, shell
completion, version output, guided manual API-key setup, browser or manual
login, credential replacement, identity/status inspection, a local effective authentication
environment report, local logout, doctor readiness checks, local MCP stdio serving,
public health/error catalog checks, and functional raw API paths.
Authenticated team Credits balance and cursor-paginated transaction history are
active as well. Local profile and non-secret configuration
commands are active without requiring credentials or contacting the API.
Authenticated raw
GET/POST/PATCH/DELETE commands are active for API-relative paths.

## Install From Source

Until release artifacts are published, this guide intentionally uses
source checkout commands rather than an archive installer.

With no saved profile or URL override, the historical profile name `local`
targets `https://www.chab.ai` for browser authorization and
`https://www.chab.ai/v1` for REST. It does not mean localhost. Existing saved
profiles retain their destinations. A legacy saved profile with no URL (or an
auth-only legacy installation) still targets localhost until you choose a URL.
Profiles saved with the former `app.chab.ai` browser origin keep the matching
`api.chab.ai/v1` API origin for credential safety; inspect with `chab auth env --plain`
and set both new origins explicitly when migrating that profile.
For a custom or staging app URL, use
`--base-url`; supply `--api-base-url` too only when that installation uses a separate API origin.
Browser login with only a custom API URL is refused; set the matching app
origin with `--base-url` as well.
You can inspect the effective pair offline with `chab auth env --plain`.

From the repository root:

```bash
go install ./cmd/chab
```

Put the selected `GOBIN` or Go bin directory on `PATH`. When `GOBIN` is unset,
Go installs commands under `$(go env GOPATH)/bin`.

Verify the installed command:

```bash
command -v chab
chab version
chab --help
```

For one-off development without installing, run from the repository root:

```bash
go run ./cmd/chab --help
go run ./cmd/chab version
go run ./cmd/chab version --json
go run ./cmd/chab setup --help
go run ./cmd/chab login --help
go run ./cmd/chab auth login --help
go run ./cmd/chab doctor
go run ./cmd/chab profile --help
go run ./cmd/chab config --help
go run ./cmd/chab credits --help
go run ./cmd/chab credits balance --help
go run ./cmd/chab credits transactions --help
go run ./cmd/chab health --help
go run ./cmd/chab errors --help
go run ./cmd/chab api --help
go run ./cmd/chab api get --help
go run ./cmd/chab api post --help
go run ./cmd/chab mcp --help
go run ./cmd/chab mcp serve --help
```

`chab version` runs offline. The JSON form is stable and contains exactly:
`version`, `commit`, `date`, `go_version`, `os`, and `arch`. `chab doctor`
runs without credentials and reports readiness without creating state, but it
checks compatibility at the configured API host; it is not offline.

## Install Agent Skills

The skill is optional guidance for using the CLI, not another login or server.
See [CLI, Skill And MCP](agent-access.md) to choose between direct CLI use, a
skill and local MCP. The hosted alternative is implemented in the companion
SaaS with separate OAuth consent; it needs no CLI or skill installation.

Repository-local skill entrypoints are already committed at:

- `.agents/skills/chab/SKILL.md`
- `.claude/skills/chab/SKILL.md`

For a Codex user-level skill, copy only `SKILL.md`:

```bash
mkdir -p "${CODEX_HOME:?set CODEX_HOME}/skills/chab"
cp .agents/skills/chab/SKILL.md "$CODEX_HOME/skills/chab/SKILL.md"
test -s "$CODEX_HOME/skills/chab/SKILL.md"
```

For a Claude Code user-level skill, copy only `SKILL.md`:

```bash
mkdir -p "$HOME/.claude/skills/chab"
cp .claude/skills/chab/SKILL.md "$HOME/.claude/skills/chab/SKILL.md"
test -s "$HOME/.claude/skills/chab/SKILL.md"
```

The user-level skill is standalone. `.chab-agent-skill/` contains maintainer
support material for this repository and is not required beside an installed
skill.

## Explore Commands

Use Cobra help as the command reference:

```bash
go run ./cmd/chab help
go run ./cmd/chab help auth
go run ./cmd/chab help auth login
go run ./cmd/chab help setup
go run ./cmd/chab help credits transactions
go run ./cmd/chab help health
go run ./cmd/chab help errors
go run ./cmd/chab help mcp serve
```

The current auth/readiness paths are functional:

```bash
go run ./cmd/chab setup
go run ./cmd/chab setup --plain
go run ./cmd/chab login
go run ./cmd/chab login --web
go run ./cmd/chab login --api-key
go run ./cmd/chab whoami
go run ./cmd/chab auth status
go run ./cmd/chab auth env
go run ./cmd/chab logout
go run ./cmd/chab doctor
go run ./cmd/chab health
go run ./cmd/chab errors
go run ./cmd/chab profile list
go run ./cmd/chab config path
go run ./cmd/chab mcp --help
```

`chab health --json` reports `free_mode.state` and each family's
`covered_operation_keys` as well as its health state. A healthy family does not
necessarily cover every operation in that family; check the listed keys before
assuming a specific free operation can start. The same public fields are
available through local MCP's `chab_health` tool without a credential.

Profile and configuration commands manage local state only:

```bash
go run ./cmd/chab profile list
go run ./cmd/chab profile show
go run ./cmd/chab profile create staging --base-url https://staging.example.test
go run ./cmd/chab profile use staging
go run ./cmd/chab config list
go run ./cmd/chab config get profiles.staging.api_base_url
go run ./cmd/chab config set profiles.staging.locale de
go run ./cmd/chab profile delete staging --yes
```

Inspection supports human, plain, JSON, jq, and template output. Mutation
commands use human/plain output; inherited `--json` keeps human output, while
`--jq` and `--template` are rejected before local files are read or written.
Stored auth is reported only through non-secret metadata. `CHAB_API_KEY` affects
API commands but does not change profile inspection; use `chab auth status` to
see the effective credential source.

Persisted profile URL edits that would change the effective API destination are
refused while that profile has stored auth. Log out that profile, update the
URL, and authenticate again. Runtime URL overrides keep their normal
flag/environment precedence.
Profile deletion is confirmation-gated, removes auth before config, and does
not revoke the server-side key.

Inspection fails without partial output when a hand-edited config contains an
invalid profile name or known profile value. An invalid stored selection can
be replaced with `chab config set current_profile <valid-name>` or
`chab profile use <valid-name>`; an explicit `--profile <valid-name>` can
bypass it for one invocation.

The config file must contain exactly one YAML document, and mapping keys must
be unique. Profile and config inspection validates every persisted profile
before printing output, so requesting one valid profile or key still fails
without partial output when a different persisted profile is invalid.

## Authorize The CLI

On a terminal, `chab login` first checks whether the resolved Chab server
supports first-party browser/device authorization. When it does, the CLI prints
a verification URL and user code to stderr, tries to open the browser, waits for
approval, validates the issued API key with `GET /v1/me`, and stores the
same auth-file record used by manual login.

Use `chab login --api-key` for manual entry, CI, SSH sessions, or older
servers. Non-terminal stdin also stays on the manual API-key path without
discovery. Use `chab login --web` to require browser login; with
`--web --no-prompt`, the CLI prints the verification URL and user code without
opening a browser.

Use a dedicated profile for an agent. Browser login uses `client_id=chab-cli`
and requests only `api:projects:read` by default. Add `--scope` for the
permissions your workflow needs, for example:

```bash
chab login --web --profile research --scope api:projects:read --scope api:search:read
```

Chab accepts 28 device scopes: all current scopes except `api:billing:write`,
`api:tokens:write`, and `api:webhooks:write`. Those three need a manually created
API key with the required scope and any applicable browser approval.
The consent screen selects only requested basic permissions initially. Choose
additional permissions explicitly, then set spending, project access, and a
1–90 day expiry. Spending and project access start at `none`; approving a paid
operation's scope does not enable spending. The CLI only persists a key after
validating its identity, approved scopes, and expiry. An older server may have
a narrower scope ceiling; update it or use a manually configured key.


## Set Up A Manual API Key

Create a team API key in the product web app, then run:

```bash
go run ./cmd/chab setup
```

Setup resolves or creates the selected profile, verifies the credential with
`GET /v1/me`, persists a copied key only after validation, and reports
the fresh team and key identity. With no usable stored key, terminal input uses
a secret prompt and non-terminal stdin is the copied key:

```bash
go run ./cmd/chab setup --profile staging --base-url https://staging.example.test
go run ./cmd/chab setup --no-prompt < api-key.txt
go run ./cmd/chab setup --plain
```

When a stored key exists, setup verifies it with one non-retried request and
never reads piped stdin. A successful check leaves the auth file byte-for-byte
unchanged while making only any necessary profile-selection or connection
repair in config. An authentication rejection can offer replacement only on
an interactive terminal. Non-interactive setup leaves both files unchanged,
returns exit `3`, and directs explicit scripted replacement to:

```bash
go run ./cmd/chab login --api-key --no-prompt --yes < api-key.txt
```

`--no-prompt` disables setup replacement even with `--yes`. Authorization,
rate-limit, protocol, transport, and other failures never enter replacement.
Setup supports human output and exactly seven `--plain` rows. Inherited
`--json` retains human output; jq, templates, response metadata, and dry-run
are unsupported.

If `CHAB_API_KEY` is set, setup and login still validate the stored, copied, or
browser-issued profile key and warn after successful output that the
environment credential takes precedence for ordinary authenticated commands.

For CI, inspect the effective non-secret invocation context before supplying
the credential from the provider's secret store:

```bash
go run ./cmd/chab auth env --profile staging --plain
go run ./cmd/chab auth env --profile staging --json
```

This applies flag and environment precedence, normalization, and URL
derivation. It differs from `profile show` and config inspection, which show
persisted file state. The command names but never looks up `CHAB_API_KEY`, does
not read the auth file, and makes no network request. Copying both URL rows
freezes the API base derivation until `CHAB_API_BASE_URL` is removed or
changed.

## Connect A Local MCP Host

`chab mcp serve` exposes local auth environment, public health and error-catalog
utilities plus operation-ID tools backed by the public Chab API contract. It
uses stdio protocol frames on stdout and redacted diagnostics on stderr. See
`chab mcp serve --help`.

Configure a host with the command, not credentials:

```bash
codex mcp add chab -- chab mcp serve
claude mcp add --transport stdio chab -- chab mcp serve
```

If the host needs an absolute executable path, resolve it from the active
shell:

```bash
codex mcp add chab -- "$(command -v chab)" mcp serve
```

For a non-default profile:

```bash
chab --profile staging mcp serve
```

Run `chab mcp serve` directly only in a protocol test harness; normal terminals
do not send the MCP frames it waits for.

Startup, discovery, `chab_auth_env`, `chab_health`, and
`chab_errors` require no credential. `tools/list` also works without a key, but
with a guest trial credential it checks live identity and the guest
compatibility list before advertising tools. API-backed tools such as `chab_auth_me`,
`chab_credits_get`, `chab_credits_transactions_list`, `chab_search_web`,
`chab_files_create`, and `chab_files_download` call the API and require either
`CHAB_API_KEY` inherited by the MCP server process or the selected profile's
auth state written by `chab login`. Login, logout, config, selected-profile,
and auth-file changes are reloaded for later calls. Changing `CHAB_API_KEY` in the host environment requires restarting or relaunching the MCP server process.

For a no-signup trial, issue the credential in Chab's browser trial page and
store it in a local profile with `chab setup` (or use `CHAB_API_KEY` from your
own secret store). Never put its bearer in a host config or chat. Guest local
MCP currently offers the advertised `chab_search_web` and
`chab_contacts_email_verify` starters, balance, identity, current-operation
helpers and own action receipts. Management, connected-account, billing,
paid-only and hosted tools remain separate; a guest REST bearer is not a
hosted MCP OAuth credential. Promotional credits are one-off, and the backend
may require a challenge, pause free usage, expire/revoke the key, or require
signup after claim.

MCP operation inputs are grouped as `path`, `query`, `body`, and `local`.
Confirmation-gated tools require `local.confirmation=true`; uploads use
`local.input_path`, downloads use `local.output_path`, and downloads refuse to
overwrite existing files. Recoverable submissions can be inspected with
`chab_action_list`, `chab_action_show`, and `chab_action_resume`.

Credits balance is a direct authenticated read. For a guest key, inspect
`free_credits.available`; `spendable_balance` is the paid team balance and is
zero. The balance read does not select or preflight a project:

```bash
go run ./cmd/chab credits
go run ./cmd/chab credits balance
go run ./cmd/chab credits balance --plain
go run ./cmd/chab credits balance --json
go run ./cmd/chab credits balance --jq .balance
go run ./cmd/chab credits balance --template '{{.balance}}'
go run ./cmd/chab credits transactions
go run ./cmd/chab credits transactions --page-size 25 --limit 250
go run ./cmd/chab credits transactions --json
go run ./cmd/chab credits transactions --include-meta --jq .meta.pagination.has_more
go run ./cmd/chab credits transactions --jq '.[].amount'
go run ./cmd/chab credits transactions --template '{{range .}}{{.id}} {{.amount}}{{"\n"}}{{end}}'
```

Without `--include-meta`, the stable value contains exact integer `balance`, nullable `subscription`, and
nullable `last_credit_update` fields. Machine output retains explicit nulls;
human and plain output omit absent optional detail rows. Missing, null, or
wrongly typed required balance fields—or required fields inside a non-null
subscription—are reported as protocol errors rather than valid-looking zero or
empty values.

Transaction `--type`, `--since`, `--until`, and `--sort` values map to the
server's `type`, `created_from`, `created_to`, and `sort` query parameters on
every fetched page. Timestamps must be whole-second RFC3339 values with `Z` or
a valid numeric offset. The CLI validates syntax but leaves inverted-window
validation and row membership to the API. Machine output without
`--include-meta` is the bare array of five-field transaction objects.

Raw API POST/PATCH/DELETE own a local offline `--dry-run`. A preview shows the method,
API-relative path, ordered query/body values when present, and
generated/explicit idempotency source without exposing a key value:

```bash
go run ./cmd/chab api post /examples/echo --field event=demo --dry-run --json
go run ./cmd/chab api patch /resources/<id> --body '{"status":"paused"}' --dry-run
go run ./cmd/chab api delete /resources/<id> --dry-run --template '{{.method}} {{.path}}'
```

The raw API family is an authenticated escape hatch for public endpoints below
the resolved API base:

```bash
go run ./cmd/chab api get /me
go run ./cmd/chab api get /credits/transactions --all --json
go run ./cmd/chab api get /credits/transactions --all --json --include-meta
go run ./cmd/chab api get /credits/transactions --raw --jq .data
go run ./cmd/chab api post /examples/echo --field event=demo --json
```

Paths cannot be arbitrary URLs and must not contain query or fragment markers;
use repeatable `--query key=value` entries instead. GET adds no page parameters
unless pagination is active; `--all=false` remains inactive. Unsafe methods
accept optional field, inline JSON, file, or stdin bodies. Validated
inline/file/stdin JSON bytes are sent unchanged after trimming surrounding
whitespace. Real unsafe requests always use idempotency and do not ask for
confirmation. By default raw leaves print the decoded `data` value; `--raw`
selects the complete success envelope. All leaves support human, plain, JSON,
jq, and template output.

## Opt-In Response Context

The persistent `--include-meta` flag is executable on exactly:

- `credits balance` and `credits transactions`
- `api get`, `api post`, `api patch`, and `api delete`

It requires `--json`, `--jq`, or `--template`; `--plain` and human output do
not support the wrapper. It is rejected on dry-run invocations and on every
other named command. Help remains available because Cobra handles
`--include-meta --help` before execution preflight.

Without the flag, successful output is byte-for-byte the existing command
value. With it, transforms see the wrapper root:

```json
{
  "data": [{"id": "project-1"}],
  "meta": {
    "request_id": "req_123",
    "pagination": {
      "current_page": 2,
      "per_page": 100,
      "total": 125,
      "last_page": 2,
      "from": 101,
      "to": 125,
      "has_more": false
    },
    "rate_limit": {
      "remaining": 498
    },
    "retry": {
      "attempts": 3,
      "waits_ms": [0, 500]
    }
  }
}
```

Request ids, pagination, and rate limits describe the final response. Retry
attempts and waits are aggregated across every physical request, including
all fetched pages. In `--limit` mode the CLI may truncate `data`, so
`len(.data)` is independent of `.meta.pagination.total`. Decoded raw GET can
add sanitized, non-pagination server envelope fields under `meta.api_meta`.
Typed commands never do, and `api get --raw` keeps the complete selected
envelope under `data` without duplicating it into outer `api_meta`.
Request-local secret and idempotency values are removed from CLI-owned
metadata. When one matches a response token, public parsed rate-limit fields
are omitted while present raw text is redacted, matching `api_meta` primitives
become `"[REDACTED]"` strings, and matching public `meta.pagination` is
omitted. Pagination traversal uses a separate validated internal value, so
`--all` and `--limit` still fetch every required page.

API and protocol errors remain unwrapped and bypass jq/templates. Safe
`retry_after` and `rate_limit` fields are included when the response carried
them, regardless of the success metadata flag.

## Completion

Generate shell completion scripts offline:

```bash
go run ./cmd/chab completion bash
go run ./cmd/chab completion zsh
go run ./cmd/chab completion fish
go run ./cmd/chab completion powershell
```

## Global Flags

The shell accepts these root flags:

```text
--profile
--config
--auth-file
--base-url
--api-base-url
--locale
--json
--include-meta
--jq
--template
--plain
--no-color
--no-ansi
--no-pager
--debug
--no-prompt
--yes
```

`--json`, `--jq`, and `--template` change behavior for `version`, `whoami`,
`auth status`, `auth env`, `doctor`, all eight functional Project leaves,
`credits balance`, `credits transactions`, all four raw API leaves, and the
profile/config inspection leaves. `--plain` is also available for those
commands, profile/config mutations, and
`setup`, `login`, and `logout`. Setup, login, logout, and profile/config
mutations accept inherited `--json` but keep human output. Setup and login emit
the environment-shadow warning only after successful result rendering.

`--include-meta` is command-dependent as documented above. Its preflight runs
before config, credential, input, confirmation, idempotency generation, and
network work.

`--yes` accepts supported confirmations but does not provide missing input.
For repeatable scripted login to an existing profile, pipe the key and pass
`--api-key --no-prompt --yes`.

## Offline Contract

Help, version, and completion do not read config files, auth files, environment
credentials, pager settings, or the network. Profile/config, Project, Credits,
and raw API command help is offline. Profile/config execution and `auth env`
are local-only and make no HTTP request. Auth/readiness, Project, Credits reads,
and real raw
requests use local state and call the API only when their command contract
requires it. A valid Project
create/update/pause/resume/archive/delete or raw POST/PATCH/DELETE dry-run is
fully offline even when local config or auth state is malformed.
