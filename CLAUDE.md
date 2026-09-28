# Chab CLI

Product Go CLI for Chab. The target binary and standalone skill name is `chab`.
The companion backend is `/home/vincent/vilt-saas-chab-ai` on branch `chab-ai`.
Its public `/v1` API and root OAuth device flow are the canonical contract.

`main` is the reusable CLI boilerplate: shared code, placeholder identity,
example functionality and framework docs. Actual SaaS CLIs, skills and local
MCP adaptations live in product branches/worktrees paired with the backend's
product branch. Check `git branch --show-current` before editing. Keep product
branding and domain behavior in those branches; bring reusable fixes to `main`.

## Framework Documentation

Start at `framework/README.md` for product adaptation, command/tool authoring
and release ownership. Customer installation belongs in `docs/`; generated
command reference belongs in `manual/commands/`. Keep these linked to the
companion backend framework docs.

## Planning

Planning lives in `.vroni/wip/`.

- `.vroni/wip/notes/vision.md` explains the product destination.
- `.vroni/wip/notes/architecture.md` records the technical decisions.
- `.vroni/wip/projects/` contains ordered implementation phases.
- `.vroni/wip/tickets/inbox/` contains implementation-ready tickets.

Tickets must be actionable implementation handoffs. Do not create tickets for
research, status reporting, decisions, setup-only work, or vague cleanup.
Resolve decisions in notes first, then write tickets that build concrete
behavior.

## Product Authority

This worktree and its independent `.vroni` belong to Chab. Keep backend and
boilerplate main read-only. The coordinating Codex session owns backend changes.
Vroni uses project oneshot quick mode with GPT-5.5 driver and Astra plus Opus
navigators, local commits only and no automatic sync. Do not push, tag, create
remote repositories, publish, deploy, contact live providers or send messages.
Use mock servers, dummy credentials and owned temporary paths for worker QA.
Do not open browsers from the Vroni worker; the operator owns host acceptance.

The notes under `.vroni/wip/notes/framework-baseline/` retain historical framework
context. Active Chab notes one directory above define the product behavior.

## Implementation Direction

- Language: Go.
- CLI framework: Cobra.
- Config/auth: explicit first-party packages, not Viper.
- Public API auth: scoped Chab team API token obtained through root OAuth device login or manual entry.
- Primary API surface: `/v1`; identity is `/me` and credits balance is `/credits`.
- Preserve Chab's top-level operation envelope and flat cursor metadata.
- Identity: `CHAB_*`, OS config directory `chab/`, MCP tools `chab_*`.
- Human output is table/detail oriented by default.
- Machine output uses stable JSON, with jq/template helpers for supported
  active commands.
- Every command and subcommand needs useful `--help`.

## Expected Checks

Use:

```bash
go test ./...
go test -count=1 ./internal/docscheck ./internal/installcheck ./internal/releasecheck ./internal/isolationcheck
go test -race ./...
go vet ./...
GOTOOLCHAIN="$(go env GOVERSION)" go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
go fmt ./...
go run ./cmd/chab --help
go run ./scripts/generate-command-docs --check
go run ./scripts/generate-operation-map --check
go run ./scripts/check-examples
go build -o /tmp/chab-local-qa/chab ./cmd/chab
scripts/smoke-release-no-secret.sh --bin /tmp/chab-local-qa/chab
```

Rungrad conformance is inactive for the current command shell; do not run an
`internal/rungradcheck` gate unless that package is restored with real tests.
