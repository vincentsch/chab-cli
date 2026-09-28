# SaaS CLI Authoring

Start with the [CLI framework index](../framework/README.md) and
[command/tool authoring workflow](../framework/command-and-tool-authoring.md).
This page retains the detailed command/output contracts.

See the [CLI, skill and MCP strategy](agent-access.md) for repository
ownership and product adaptation. This guide covers the command implementation
patterns; hosted MCP belongs in the companion SaaS backend.

The current shell includes the command boundary, catalog, help, completion,
version output, manual API-key auth, identity/status commands, readiness
checks, a strict-runtime-only authentication environment report, shared output
rendering, JSON/plain/jq/template output controls, generated manual command
pages, test harness, local exit classification, and direct authenticated
team-level credits balance and cursor-paginated transaction reads with stable human, plain,
JSON, jq, and template output. Native profile/config commands add local-only
inspection and mutation, stable inspection values, config/auth preservation,
stored-credential redaction, persisted destination-change protection, and
confirmation-gated deletion. Native raw API commands add a controlled
API-relative escape hatch with repeatable query values, shared pagination,
optional JSON bodies with exact validated inline/file/stdin byte replay,
offline unsafe-method previews, idempotency, five output modes, and
credential/request-secret scope separation. The catalog-controlled response
metadata capability wraps successful machine values on request while keeping
default output stable. Reusable feature-module
architecture remains outside the runnable surface.

Use Cobra help to inspect the stable command paths:

```bash
go run ./cmd/chab help
go run ./cmd/chab help auth env
go run ./cmd/chab help credits transactions
go run ./cmd/chab help profile
go run ./cmd/chab help config
go run ./cmd/chab help health
go run ./cmd/chab help errors
go run ./cmd/chab help api
go run ./cmd/chab help api get
go run ./cmd/chab help api post
go run ./cmd/chab help version
```

The generated command reference is checked in under
[manual/commands/README.md](../manual/commands/README.md).

## Response Metadata Capability

`CommandSpec.SupportsMeta` is the runtime support inventory.
`Sidecar.HelpRequirements` must contain `HelpMetadata` exactly when
`SupportsMeta` is true. Do not infer this capability from generic JSON support:
local inspection, version, `setup`, `auth env`, `whoami`, `auth status`, and
`doctor` intentionally remain unsupported.

Supported commands pass their original machine value and `api.ResponseMeta` to
`Factory.WriteResultWithMeta`. The shared writer derives JSON/jq/template
support from the original value, then wraps only when the parsed
`--include-meta` flag is active. This preserves default bytes and prevents
metadata from making a human-only result machine-capable. The command call
site owns `includeAPIMeta`: decoded raw GET opts in, while typed commands,
unsafe raw methods, and full `--raw` envelopes do not.

The root preflight rejects metadata on unsupported named paths, then rejects an
active local dry run, then requires JSON, jq, or template. This must happen
before command callbacks read config, credentials, input, confirmation state,
or the network.

Success metadata is a `{data, meta}` wrapper. API and protocol failures use the
central error renderer instead: they are never wrapped or transformed, and
safe Retry-After/rate-limit fields are shown independently of the success
flag.

Response metadata is sanitized inside the API client with the request-local
redactor after retry decisions use the original headers and before any
success/typed-error handoff. CLI-owned metadata uses credential,
request-marked, and resolved idempotency-key scope. API-owned raw success data
retains its narrower credential-only dynamic scope plus static structured
redaction. Bearer and bootstrap clients both sanitize populated raw envelope
metadata fail-closed with the shared ordered JSON transform. Matching raw
metadata primitives become marker strings; parsed header fields and public
pagination are omitted when their source requires redaction. The private
validated carrier exposed through
[`PaginationForTraversal`](../internal/api/meta.go) keeps `--all` and
`--limit` execution independent of the public sanitized copy.
