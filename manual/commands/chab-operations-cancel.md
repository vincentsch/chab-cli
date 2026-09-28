# chab operations cancel

## Usage

```text
chab operations cancel <operation-id> [flags]
```

## Description

Cancel an operation cooperatively.

Cancellation changes remote operation state and requires explicit
acknowledgement through --yes or an interactive confirmation. The local action
record is created before the cancel request and uses one stable idempotency key.
Use --plain for the accepted operation id or action id, and --json for the
stable action and recovery object. Pass --idempotency-key to supply a
caller-owned replay key. In --no-prompt or non-interactive mode, use --yes to
acknowledge cancellation.

Related commands:
- [chab operations wait](chab-operations-wait.md)
- [chab operations bulk-cancel](chab-operations-bulk-cancel.md)

## Examples

```text
  chab operations cancel op_123 --yes
  chab operations cancel op_123 --yes --json
  chab operations cancel op_123 --yes --plain
  chab operations cancel op_123 --idempotency-key <key> --yes
```

## Flags

- `--idempotency-key` - explicit idempotency key; generated when omitted

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

idempotency, confirmation
