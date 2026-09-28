# Chab Agent Skill

This directory contains checked-in maintainer support material for the Chab-SaaS
agent skill. It is not a runtime dependency for installed user-level skills.

The repository skill entrypoints live at:

- `.agents/skills/chab/SKILL.md`
- `.claude/skills/chab/SKILL.md`

The entrypoint files are byte-identical and standalone. Keep workflow and safety
guidance in the entrypoints, and keep generated command reference material in
`commands.md`.

## Install

Install or verify the `chab` binary before using the skill:

- `chab version`
- `chab --help`

User-level skill installation copies only `SKILL.md` into the host-specific
skill directory. Do not copy this support directory as required runtime
material.

Manual skill destinations are documented in `docs/install.md`:

- Codex repository skill: `.agents/skills/chab/SKILL.md`
- Codex user skill: `$CODEX_HOME/skills/chab/SKILL.md`
- Claude repository skill: `.claude/skills/chab/SKILL.md`
- Claude user skill: `~/.claude/skills/chab/SKILL.md`

The checksum manifest covers the committed skill files for maintainer audits.
Verify it after editing the skill entrypoints or command reference.

## MCP

This material intentionally does not write Codex, Claude, or MCP host
configuration. To connect the local MCP server, run one of these commands in the
target environment:

```sh
codex mcp add chab -- chab mcp serve
claude mcp add --transport stdio chab -- chab mcp serve
```
