# chab version

## Usage

```text
chab version [flags]
```

## Description

Print build metadata for the chab binary.

With --json, output is a stable JSON object with exactly these fields:

```text
version     release version, or "dev" for local builds
commit      source commit, or "none" for local builds
date        build date, or "unknown" for local builds
go_version  Go runtime version
os          runtime operating system
arch        runtime architecture
```

The same JSON shape can be filtered with --jq or rendered with --template.
Use --plain for copy-safe key/value rows.

Related commands:
- [chab doctor](chab-doctor.md)
- [chab completion](chab-completion.md)

## Examples

```text
  chab version
  chab version --json
  chab version --jq .version
  chab version --template '{{.version}}'
  chab version --plain
```

## Output modes

human, JSON, plain, jq, template

## Authentication

Runs without requiring a credential.
