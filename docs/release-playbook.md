# Release Playbook

For product identity, skill packaging and the complete maintenance path,
start with [distribution and maintenance](../framework/distribution-and-maintenance.md).
The workflow packages five binary archives and a standalone agent-skill zip,
all covered by the release checksum file. Publishing belongs to the product's
configured release destination; it is not required to start a product branch.

This playbook covers the current local, no-secret release checks for `chab`.

The shell can be built and smoked locally:

```bash
go build ./cmd/chab
go run ./cmd/chab --help
go run ./cmd/chab version --json
go run ./cmd/chab doctor --help
go run ./cmd/chab profile --help
go run ./cmd/chab profile list --json
go run ./cmd/chab config --help
go run ./cmd/chab config path
go run ./cmd/chab login --help
go run ./cmd/chab auth login --help
go run ./cmd/chab auth env --json
go run ./cmd/chab credits --help
go run ./cmd/chab credits balance --help
go run ./cmd/chab credits transactions --help
go run ./cmd/chab health --help
go run ./cmd/chab errors --help
go run ./cmd/chab api --help
go run ./cmd/chab api get --help
go run ./cmd/chab api post /examples/echo --field event=demo --dry-run --json
go run ./cmd/chab completion bash
go run ./scripts/generate-command-docs --check
go run ./scripts/generate-operation-map --check
go run ./scripts/check-examples
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT
go build -o "$tmpdir/chab" ./cmd/chab
scripts/smoke-release-no-secret.sh --bin "$tmpdir/chab"
```

The no-secret smoke below executes doctor against a closed loopback port;
running doctor normally contacts the configured API even without a key.
The focused commands cover local profile/config help and inspection,
Credits, public system checks, raw API help, and offline raw write previews.
The no-secret smoke covers read-only local paths and effective
authentication-environment resolution plus Credits family help, but does not
perform live Credits balance or transaction reads, or credentialed mutations.
The checked automation workflows cover the effective environment and resource operations against local mocks. The
checker owns the current workflow inventory.

## Snapshot Archive Validation

Release-ready archive validation is maintainer-only until a release is
published. It requires GoReleaser v2.16.0 on `PATH`.

Run:

```bash
goreleaser check
go run ./scripts/release-snapshot-smoke
```

The snapshot smoke runs `goreleaser release --snapshot --clean`, then verifies
the complete supported archive matrix:

```text
chab_0.0.0_linux_amd64.tar.gz
chab_0.0.0_linux_arm64.tar.gz
chab_0.0.0_darwin_amd64.tar.gz
chab_0.0.0_darwin_arm64.tar.gz
chab_0.0.0_windows_amd64.zip
chab_0.0.0_agent.zip
chab_0.0.0_checksums.txt
```

It checks checksum-file coverage and digest correctness for every archive,
requires one regular top-level binary member per binary archive, rejects unsafe
member shapes, verifies the skill archive contains exactly `chab/SKILL.md` with
bytes matching the checked standalone entrypoint, and manually extracts every
binary target including the Windows zip, executes the current-platform archive binary with
`version --json`, serves the snapshot assets through a local GitHub-shaped
release server, installs the current POSIX archive through `scripts/install.sh`,
checks the installed binary's executable bit and `version --json`, and runs
`scripts/smoke-release-no-secret.sh --bin <installed-chab>`.

The POSIX installer intentionally does not install Windows archives. Windows
coverage in this local lane is checksum verification plus manual zip extraction;
native Windows execution remains a separate runner concern.

## Standalone Skill Artifact

`chab_VERSION_agent.zip` contains only `chab/SKILL.md`. GoReleaser creates this
portable archive in the same run as the binaries and includes its SHA-256 in
`chab_VERSION_checksums.txt`. The existing tag workflow uploads both. The archive
has no CLI binary, maintainer reference dependency or host-configuration script.

After obtaining the matching archive and checksum file from the product release,
verify the zip checksum before extracting it into a temporary directory. Copy
its `chab/SKILL.md` into the chosen host's skill destination using the existing
[installation instructions](install.md). Install the binary separately. Product
branches must adapt the archive name and internal skill directory along with
both source entrypoints. Snapshot validation checks the actual packaged bytes;
the docs tests verify both source entrypoints agree and their manifest is current.

## Published Install Gate

Do not publish a release, add install instructions, or claim published
installation until all of these are true:

- the repository is public or otherwise reachable by intended installers;
- a stable `vMAJOR.MINOR.PATCH` tag is pushed;
- the release workflow completes with the snapshot installer smoke and the real
  publish step;
- all five binary archives and the checksum file are uploaded and match the
  archive matrix above with the real version;
- the installer is fetched from a pinned tag or pinned release asset for the
  same version being installed;
- `chab_VERSION_agent.zip` is available and covered by the release checksum file;
- installation has been verified from the actual published release, not only from
  a local snapshot server.

The binary archives do not contain `scripts/install.sh`. Public install docs
must either fetch that installer from the matching tag source or from a separate
pinned release asset before running it. The installer may resolve `latest` only
when a caller does not pass `--version`; published docs should prefer a pinned
version or pinned asset URL.

Published release archives, update checks, and credentialed release checks are
outside the automatic local no-secret boundary. Source checkout no longer needs
a private `rungrad` module token because `github.com/vincentsch/rungrad` is
public, but this repository remains a maintainer/collaborator flow while it is
private. Do not advertise `go install` as a public install path until repository
or release reachability and installation from the actual published artifact have
been verified.

Credentialed Credits transaction reads and raw API execution remain explicitly
gated live workflows; neither is added to the automatic no-secret smoke.
