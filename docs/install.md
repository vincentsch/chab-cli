# Install Notes

Chab is distributed from the public [Chab CLI repository](https://github.com/vincentsch/chab-cli).
These full-beta installation instructions are pinned to version **0.1.1**. The app and CLI use the same
`https://www.chab.ai` origin. The CLI does not automatically update itself.

## Install The Binary

Download the installer from the matching version, inspect it, then run it:

```bash
curl -fsSLo /tmp/chab-install-v0.1.1.sh https://raw.githubusercontent.com/vincentsch/chab-cli/v0.1.1/scripts/install.sh
less /tmp/chab-install-v0.1.1.sh
sh /tmp/chab-install-v0.1.1.sh --version 0.1.1
export PATH="$HOME/.local/bin:$PATH"
chab version --json
```

The installer verifies the archive against the release SHA-256 checksum file
before installing the binary. Linux and macOS have amd64/arm64 archives;
Windows amd64 users can download and verify the zip from the
[versioned release](https://github.com/vincentsch/chab-cli/releases/tag/v0.1.1).
The `chab_0.1.1_agent.zip` skill archive has its own checksum in that release.
Extract it into a temporary directory and copy `chab/SKILL.md` to one of the
destinations below; the binary is installed separately.

A fresh CLI profile named `local` is a historical profile label, not a
localhost target: without a saved URL override it uses
`https://www.chab.ai` for browser authorization and
`https://www.chab.ai/v1` for REST. Existing saved profiles keep their URLs; a legacy
saved profile that omitted its URL, or an auth-only legacy installation, keeps
the old localhost destination until you set a URL explicitly.
Existing saved profiles are never silently pointed at a new server.

Maintainers can validate snapshot archives and the installer with
[docs/release-playbook.md](release-playbook.md).

## Install From A Source Checkout

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

For one-off development without installing, run from a checkout:

```bash
go run ./cmd/chab --help
go run ./cmd/chab version
go run ./cmd/chab version --json
go run ./cmd/chab doctor
go run ./cmd/chab completion bash
go run ./cmd/chab mcp --help
go run ./cmd/chab mcp serve --help
```

`chab version` is offline. `chab doctor` can run without credentials but checks
API compatibility at the configured host; it is not an offline command.
`chab mcp serve` is available as a local stdio server for
agent hosts once a binary is on `PATH`; host configuration must point at the
command and must not contain credentials. See
[getting started](getting-started.md#connect-a-local-mcp-host) for absolute
executable path host configuration.

Supported skill destinations:

- repository Codex: `.agents/skills/chab/SKILL.md`
- user Codex: `$CODEX_HOME/skills/chab/SKILL.md`
- repository Claude: `.claude/skills/chab/SKILL.md`
- user Claude: `~/.claude/skills/chab/SKILL.md`

User-level skill installation copies only `SKILL.md`. Maintainer support
material and checksums remain under `.chab-agent-skill/`. The repository does
not auto-write Codex, Claude, or MCP host configuration.

The shell does not implement update checking.
