# chab operations show

## Usage

```text
chab operations show <operation-id> [flags]
```

## Description

Show operation status without treating a failed remote operation as a local command failure.

With --json, --jq, or --template, --include-meta wraps the operation status
under data and safe response metadata under meta. Use --plain to print the raw
status value.

Related commands:
- [chab operations wait](chab-operations-wait.md)
- [chab operations result](chab-operations-result.md)

## Examples

```text
  chab operations show op_123
  chab operations show op_123 --json
  chab operations show op_123 --json --include-meta
  chab operations show op_123 --jq .status
  chab operations show op_123 --template '{{.status}}'
  chab operations show op_123 --plain
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
