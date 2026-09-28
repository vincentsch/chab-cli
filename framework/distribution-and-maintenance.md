# Distribution And Maintenance

[Framework index](README.md) · [Product adaptation](product-adaptation.md)

Source installation is useful for development. A customer release also needs
reachable downloads, a matching skill, tested installation and a documented
update path. The exact archive/installer commands and release criteria live in
the [release playbook](../docs/release-playbook.md).

## Shared Machinery And Product Responsibilities

| Area | Exists | Product work still required |
| --- | --- | --- |
| CLI binary | Go build, version linker flags, GoReleaser and tag workflow | Product identity, supported platforms and published install acceptance |
| Archives | Linux/macOS amd64/arm64 and Windows amd64, checksums, snapshot/installer smoke | Product names/repository, real downloaded artifact checks; native Windows execution on a suitable runner |
| Installer | Retained POSIX installer with snapshot validation | Reachable pinned installer/download location for the product |
| Skill | Two standalone entrypoints, generated reference, checksum manifest, versioned zip included in release checksums | Adapt skill identity/workflows and verify the product download |
| Local MCP | Included in the binary | Host examples with correct executable/profile and real host acceptance |
| Hosted MCP | Implemented in the backend | Product deployment, OAuth configuration and host acceptance; no CLI release dependency |
| Updates | Git history and ordinary replacement installation | Supported-version policy and documented manual update; no automatic update checker |

The boilerplate needs no published release to be ready for product worktrees.
Customer distribution is configured in each product branch. Worktrees share
Git tags and remotes, so choose a deliberate publishing destination and tag
strategy; the reference `v*` workflow is not automatically isolated per product.
A product can use public or authenticated distribution, but intended customers
must actually be able to obtain it.

## Release Sequence

1. Record backend/API compatibility, CLI version and matching skill version.
   Preserve the same workflow/permission meaning between supported releases.
2. Run root `CLAUDE.md` / CI checks: full Go tests, uncached file-backed docs,
   install/release/isolation checks, race tests, vet, vulnerability scan, help
   and generated-reference checks, checked examples and offline smoke.
3. Run the playbook's GoReleaser configuration and snapshot installer checks.
   Review the actual archive contents and checksums, not only build success.
4. Verify the packaged product skill and its distribution. Keep the installed
   `SKILL.md` standalone; maintainer reference files are not required beside it.
   Supply versioned downloads and checksum verification without auto-writing
   a customer's host configuration.
5. Publish through the product's release process. Verify installation from the
   actual published assets in a clean environment, including customer access.
6. Complete one useful CLI/skill/local-MCP workflow against staging. Hosted
   OAuth/tool/revocation acceptance is a separate backend deployment check.
7. Publish product-specific install, upgrade, connection and removal docs.
   Link to the matching release manual and explicitly state unsupported tools.

Native binaries and copied skills do not update when the repository changes.
Document how to replace each and when to restart a local MCP host process.
The installed skill must not guide users toward commands absent from their
supported binary version.

## Compatibility And Upstream Fixes

Maintain one product record of both upstream SHAs and released versions. For
each upgrade, review API shapes, output/error contracts, tool IDs/schemas,
auth behavior and skill instructions. Test old supported CLI with new backend
and new CLI against old supported backend. An absent feature must fail clearly;
never silently switch to broader credentials or a different endpoint.

Merge or selectively backport reviewed `main` fixes into each product branch,
run relevant checks and release the changed artifacts. CLI and backend releases
need not share version numbers, but compatibility must be explicit. A hosted
tool rename or semantic expansion can require new consent; old grants must not
be reinterpreted to authorize more.

During downstream maintenance, record which shared fixes each product has
adopted and replace released binaries/skills when needed. Keep product operation
maps in backend product docs and link them from the product CLI; keep private
QA evidence in `.vroni/wip/`. These are product responsibilities, not unfinished
boilerplate implementation.
