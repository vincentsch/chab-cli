# chab operations wait

## Usage

```text
chab operations wait <operation-id> [flags]
```

## Description

Wait for an operation to reach a terminal state.

The command uses the server long-poll query with waits capped at thirty
seconds. Interrupting this command stops only local waiting; it does not cancel
remote work.
With --json, timeout or interruption writes a recovery object before returning
nonzero. With --json, --jq, or --template, --include-meta wraps successful
terminal operation status under data and safe response metadata under meta. Use
--plain for raw terminal status output.

Related commands:
- [chab operations cancel](chab-operations-cancel.md)
- [chab operations result](chab-operations-result.md)

## Examples

```text
  chab operations wait op_123
  chab operations wait op_123 --timeout 2m --json
  chab operations wait op_123 --json --include-meta
  chab operations wait op_123 --jq .status
  chab operations wait op_123 --template '{{.status}}'
  chab operations wait op_123 --plain
```

## Flags

- `--timeout` - maximum local wait duration

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
