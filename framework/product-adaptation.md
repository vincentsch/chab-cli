# Adapt The CLI To A Product

[Framework index](README.md) · [Authoring](command-and-tool-authoring.md) ·
[Distribution](distribution-and-maintenance.md)

There is no single product manifest that rebrands this runtime. Renaming the
executable alone leaves `VILT_*`, credentials, tools and release artifacts
pointing at the reference identity. This checklist is performed in the product's
CLI worktree. The boilerplate keeps its working reference identity on `main`.

## Preserve History And Define Identity

Create a product branch/worktree of this repository, paired with the same
product's backend branch/worktree. Follow `framework/agent-access-launch.md` in that backend worktree.
Record both full `main` base commits. A separate product repository is optional
for publishing and is not the default development requirement.

Worktrees share Git remotes and tags: setting a remote here affects companion
worktrees too, and a `v*` tag is not scoped to your product branch. Choose the
product's release destination and workflow in its branch before publishing.
Keep shared fixes separate from product policy changes and merge reviewed
`main` updates into the product branch.

Choose a binary/skill name, Go module, environment prefix, configuration
directory, production/staging destinations, MCP server/tool names and release
repository. Record the choices in the product backend's operation/compatibility
document and link to it here. The names below are current reference values.

## Identity Checklist

| Surface | Source to inspect / change | Acceptance |
| --- | --- | --- |
| Go module and executable | `go.mod`, imports, `cmd/vilt/`, build/CI scripts | Product executable builds; module and linker paths agree |
| CLI name/help/catalog | `internal/cli/root.go`, `catalog.go`, `internal/commands/`, help goldens | Root/subcommand help and errors consistently name the product |
| Defaults and state paths | `internal/config/types.go`, `resolve.go`, config tests | Product URL is deliberate; default config/auth paths have a product-specific directory |
| Environment | `internal/config/`, `internal/auth/`, `internal/runtimeflags/`, `internal/cmdutil/`, `internal/redact/` | Product variables resolve and redact correctly; reference variables do not affect the product |
| API identity | `internal/api/client.go`, `bootstrap.go`, `internal/commands/authcmd/` | User-Agent and browser/device client labels identify the product |
| Local MCP | `internal/mcpserver/server.go`, Mail adapters, descriptions/tests | Server identity, tool prefixes, descriptions and errors match product |
| Skill | Both `SKILL.md` entrypoints, `.chab-agent-skill/`, generator | Product-named standalone skill copies are identical and checksummed |
| Packaging | `.goreleaser.yaml`, `scripts/install.sh`, snapshot smoke, `.github/workflows/`, release/install tests | Binary, archive, checksums, repository and installer names agree |
| Docs/examples | `README.md`, `docs/`, `framework/`, `examples/`, generators | Links/defaults/help point at the product and its tested release |

Reference state lives under `os.UserConfigDir()/vilt/`, with `config.yml` and
`auth.json`. Override paths exist, but requiring every customer to override
them is not a finished product identity. Reference variables include
`VILT_API_KEY`, `VILT_CONFIG`, `VILT_AUTH_FILE`, `VILT_PROFILE`, `VILT_BASE_URL`,
`VILT_API_BASE_URL`, `VILT_LOCALE` and `VILT_PAGER`. Find other references rather
than treating this list as a global replacement script:

```sh
rg -n --hidden -g '!.git/**' -g '!go.sum' -e VILT_ -e chab-cli -e 'vilt/' -e vilt_ cmd internal scripts .github .goreleaser.yaml docs framework .agents .claude .chab-agent-skill
```

Preserve generic `PAGER` behavior unless intentionally changing its contract.
Do not globally replace protocol names, public API fields or historical
upstream references. Regenerate manuals and skill command reference from their
generators. If the Go module changes, update GoReleaser's `-X` version/commit/date
linker paths as well.

This Chab worktree defaults a truly fresh profile to the Chab app/API HTTPS
host pair; legacy URL-omitting saved profiles and auth-only stored-key state
retain localhost until explicitly changed. Other product worktrees must choose
their own defaults or require explicit setup. Profiles are
destinations, not security boundaries. Verify URL changes with stored auth still
require the existing logout/re-authentication flow.

## Choose The Product Surface

Retain only useful command families. Root registration and catalog metadata are
explicit, and local MCP has its own registry. Removing a command does not remove
its MCP tool. OpenAPI is not used to automatically generate native commands.
Keep the raw REST escape hatch only if its authorization and user experience
are appropriate for the product.

The current browser login requests the human `cli` client kind. It does not
offer the backend's scoped `local_agent` configuration as a CLI option. Use a
manually created scoped/expiring key and a dedicated profile for limited local
agent use. Local tool annotations do not narrow the underlying API key.

Hosted MCP branding happens separately in the backend: application name and
canonical origin are configurable; tool IDs and consent/catalog mappings still
contain explicit reference names. Renaming a tool ID changes what stored grants
approve. Plan compatibility or new consent; never reinterpret an old approval
as access to a different operation.

## Prove Isolation Before Shipping

Use two built product binaries in the **same temporary OS configuration root**
and separate mock API endpoints. This proves the product namespaces themselves
prevent collisions; separate roots alone would hide a failed rename. Use no
personal credentials. Also test any explicit config/auth path overrides.

| Scenario | Required result |
| --- | --- |
| Default `config path` for both products | Distinct product directories and credential files |
| Only product A's variables set | Product B ignores A's key, paths, profile and URLs |
| Login/logout/profile deletion in A | B's files remain byte-identical |
| URL/profile change while logged in | Existing destination-change protections still apply |
| Help/version/completion with malformed config and hostile variables | No config/auth read, network access, secret output or state creation |
| Local MCP entries for A and B | Correct binary/profile, product tools and API destination |
| Both standalone skills installed | Distinct skill names and accurate product command guidance |
| Installed archive binary | Build metadata, defaults and paths match source-built product |

`internal/isolationcheck` proves reference isolation properties; it does not
prove a future product rename. Adapt those fixtures and run the real two-product
exercise. Preserve its sanitized results with both baseline commits.
