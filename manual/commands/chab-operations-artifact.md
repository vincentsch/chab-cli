# chab operations artifact

## Usage

```text
chab operations artifact <operation-id> <artifact-id> [flags]
```

## Description

Fetch metadata for a customer-visible operation artifact.

Use the download subcommand to write artifact bytes to an explicit new local
path. Metadata and byte downloads both resolve server-provided resource links
below the configured API base.
With --json, --jq, or --template, --include-meta wraps artifact metadata under
data and safe response metadata under meta. Use --plain for raw metadata
output.

Related commands:
- [chab operations result](chab-operations-result.md)
- [chab operations artifact download](chab-operations-artifact-download.md)

## Subcommands

- [chab operations artifact download](chab-operations-artifact-download.md) - Download operation artifact bytes

## Examples

```text
  chab operations artifact op_123 art_123
  chab operations artifact op_123 art_123 --json
  chab operations artifact op_123 art_123 --json --include-meta
  chab operations artifact op_123 art_123 --jq .id
  chab operations artifact op_123 art_123 --template '{{.id}}'
  chab operations artifact op_123 art_123 --plain
  chab operations artifact download op_123 art_123 --output ./result.md
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
