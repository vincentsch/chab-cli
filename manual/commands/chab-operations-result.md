# chab operations result

## Usage

```text
chab operations result <operation-id> [flags]
```

## Description

Fetch the retained result for a completed operation.

If the operation succeeded with result_available=false, Chab may return a
not-found or not-ready API error. Artifact bytes are handled by a later file
transfer workflow; this command returns result metadata and inline results.
With --json, --jq, or --template, --include-meta wraps the operation result
under data and safe response metadata under meta. Use --plain for raw result
output.

Related commands:
- [chab operations wait](chab-operations-wait.md)
- [chab operations artifact](chab-operations-artifact.md)

## Examples

```text
  chab operations result op_123
  chab operations result op_123 --json
  chab operations result op_123 --json --include-meta
  chab operations result op_123 --jq .result
  chab operations result op_123 --template '{{.result}}'
  chab operations result op_123 --plain
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
