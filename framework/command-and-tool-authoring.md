# Author Commands, Skills And Local MCP Tools

[Framework index](README.md) · [Product adaptation](product-adaptation.md)

Define backend policy and REST behavior first. CLI/local MCP call that API;
hosted MCP has a separate backend adapter and consent contract. Record the
customer workflow and deliberate omissions in the product backend's operation
map. A command, skill and two MCP tools are not generated from one schema.

## Source Map

| Concern | Reference implementation |
| --- | --- |
| Native command wiring | `internal/cli/root.go`, `internal/commands/projects/`, `credits/`, `mailcmd/` |
| Command metadata/output support | `internal/cli/catalog.go` and catalog tests |
| Shared runtime | `internal/cmdutil/`, `internal/config/`, `internal/auth/`, `internal/api/` |
| Read services | `internal/readservice/` |
| Mail mutation services | `internal/mailservice/` |
| Local MCP registration/schemas | `internal/mcpserver/server.go`, `mail.go`, `mail_write.go` and other Mail adapters |
| Safe output | `internal/output/`, `internal/redact/`, `internal/outputtransform/` |
| Help/manual drift | `internal/cli/help_golden_test.go`, `scripts/generate-command-docs/` |
| Skill reference/checksums | `scripts/generate-agent-reference/`, `internal/docscheck/` |

See [detailed command authoring](../docs/saas-cli-authoring.md) for output and
response-metadata contracts. Copy a close working pattern, not a generic raw
HTTP call embedded inside a new command.

## Add A Read

Use Project list/show as a complete vertical example:

1. Backend owns `/api/v1/projects`, authorization and stable resource output.
2. A shared read service validates the response and uses the common API client.
3. A Cobra command owns arguments/flags and calls the service through the
   factory. Add root registration, catalog metadata, useful help and examples.
4. If local MCP should expose it, add an explicit schema/handler in its
   registry, using the shared service rather than invoking a CLI subprocess.
5. If hosted MCP should expose it, implement the PHP adapter, effective
   authority and consent mapping in the backend. See the hosted MCP section of `framework/agent-access-launch.md` in the paired backend worktree.
6. Update the skill only where a workflow needs guidance. Regenerate reference
   material and test denial paths as well as successful data.

Prove pagination/filter behavior, malformed backend data, empty results,
inaccessible resources, output modes, stderr separation and errors. Help must
remain offline. Confirm selected profile/team behavior in a real host; local
discovery is not proof that a call is authorized.

## Add A Mutation

Project writes show strict, presence-preserving input, offline previews and
idempotency. Project delete adds default-no confirmation. Mail demonstrates
finer authority: personal approvals, revisions, prepared send intents and
operation-specific confirmations. Reuse the right pattern for the effect.

Local MCP does not expose the Project write commands. It does expose delegated
Mail mutations. Adding a command is not sufficient to add a tool, and a tool's
`readOnlyHint` is descriptive metadata, not authorization.

Test missing approval, stale revision, duplicate/uncertain requests, validation,
destructive confirmation and secret redaction. For uncertain sends, inspect
the existing intent rather than creating a fresh send. Secret-producing routes
need their one-attempt/private-handoff rules; do not add generic retries or
idempotency just because other writes use them. Hosted writes remain separate
implementation work; do not silently expand existing OAuth approvals.

## Maintain The Skill

The two entrypoints are `.agents/skills/vilt/SKILL.md` and
`.claude/skills/vilt/SKILL.md`. They must remain byte-identical and standalone;
installed users copy only `SKILL.md`. Keep the product name/description,
workflow, credential boundaries and write safety accurate. Do not say the MCP
server is read-only when it exposes Mail mutations.

Do not embed a long static command catalog into the skill. Teach command
discovery and a few useful product workflows; generated reference material
belongs in `.chab-agent-skill/commands.md`. No copied private logs, credentials
or repository-relative runtime dependencies belong in the installed skill.

After intentional command/help or skill changes:

```sh
go test ./internal/cli -run TestHelpGoldens -update-help-golden
go run ./scripts/generate-command-docs
go run ./scripts/generate-operation-map
go run ./scripts/generate-agent-reference
go test -count=1 ./internal/docscheck
go run ./scripts/generate-command-docs --check
go run ./scripts/generate-operation-map --check
go run ./scripts/check-examples
```

Review generated diffs; do not accept an unexpected command or permission
change by regenerating snapshots. Add/update tests beside the changed command,
service and MCP adapter, then run the repository checks in
[distribution and maintenance](distribution-and-maintenance.md).

For local protocol acceptance, test initialization/discovery and actual calls
using supported hosts. Modern discovery requests have protocol metadata rules;
the server also has tested legacy initialization support. Prefer the SDK and
existing `internal/mcpserver` harness over hand-written partial JSON-RPC.
Keep stdout reserved for protocol frames and diagnostics on stderr.
