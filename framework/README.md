# CLI Framework Documentation

Developer guidance for deriving a product CLI, skill and local MCP from this
repository. `main` owns reusable code, shared features, placeholder branding
and these tracked framework docs. SaaS functionality and identity belong in
product branches/worktrees paired with the backend; the guides explain that work.
Customer installation remains in `docs/`; command reference remains generated
under `manual/commands/`.

Start with `framework/agent-access.md` (the four-interface overview) and
`framework/agent-access-launch.md` (the product launch guide) in the paired
backend branch/worktree.
This repository owns the CLI binary, standalone skill and local stdio MCP.
The backend owns REST policy and hosted HTTPS MCP. Two repos are sufficient.

## Developer Path

1. [Adapt product identity](product-adaptation.md): history, URLs, environment,
   credentials, tools, skills and side-by-side isolation.
2. [Author commands and tools](command-and-tool-authoring.md): backend-first
   feature work, source map, skill maintenance and contract checks.
3. [Distribute and maintain](distribution-and-maintenance.md): binary/skill
   release machinery, acceptance, version compatibility and upstream updates.

## Chab Product Checkout

This worktree provides Chab’s 98 API operations and 95 local MCP tools. Use the
[product README](../README.md) and [operation map](../docs/operation-map.md) for
current product coverage. Use the backend documents named above from the
branch/worktree paired with this CLI. Hosted MCP acceptance is tracked there.

## Inherited Reference Scope

The CLI implements auth/profiles, Projects, Credits, Mail and raw REST access.
Local MCP is stdio in this binary and includes delegated Mail mutations; it is
not globally read-only. At baseline `4417b25`, discovery returns 200 tools.
The skill is one standalone instruction set with Codex/Claude entrypoints.
Hosted MCP is implemented in the backend, disabled by default, with five read
tools and separate OAuth credentials. It has no dependency on this binary.

This is a reference to adapt, not a one-command generator. Identity is spread
across source and packaging, and the current runtime is not configured by one
product manifest. RunGrad scaffold tests are comparison proofs, not a generator
for this complete application. The backend remains authoritative for access.

The boilerplate includes binary/standalone-skill archive packaging, checksums
and snapshot/installer validation. It does not need a public release or a branded
product deployment before you start product worktrees. Product teams configure
their own publishing destination and acceptance. No automatic update checker exists.

## Documentation Contract

| Location | Owns |
| --- | --- |
| `framework/` | Shared developer adaptation, architecture and maintenance guidance |
| `docs/install.md`, `docs/getting-started.md` | Customer installation, login, skill installation and local MCP connection |
| `docs/agent-access.md` | Customer-facing choice of interfaces and links to developer guides |
| `docs/saas-cli-authoring.md` | Detailed command/output implementation reference |
| `docs/release-playbook.md` | Exact archive, installer and release checks |
| `manual/commands/` | Generated Cobra command reference |
| `.agents/skills/`, `.claude/skills/` | Standalone product skill entrypoints |
| `.chab-agent-skill/` | Maintainer reference and checksums; not an installed skill dependency |
| Product backend docs | Operation map, authority, API/hosted contracts and supported-version record |

Cross-repo links require access and must be changed to product links downstream.
Update both repos when availability/authentication/coverage changes. Keep one
operation map in the backend and link to it rather than copying inventories.
