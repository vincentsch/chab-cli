# chab operations bulk-cancel

## Usage

```text
chab operations bulk-cancel [flags]
```

## Description

Cancel operations by request body filters.

Pass the documented operations.bulk_cancel request body with --input. This is
a remote state-changing action and requires --yes or an interactive
confirmation.
Use --plain for the accepted operation id or action id, and --json for the
stable action and recovery object. Pass --idempotency-key to supply a
caller-owned replay key. In --no-prompt or non-interactive mode, use --yes to
acknowledge cancellation.

Related commands:
- [chab operations cancel](chab-operations-cancel.md)
- [chab operations list](chab-operations-list.md)

## Examples

```text
  chab operations bulk-cancel --input @cancel.json --yes
  chab operations bulk-cancel --input @cancel.json --yes --json
  chab operations bulk-cancel --input @cancel.json --yes --plain
```

## Flags

- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--input` - JSON request body, @path, or @- for stdin
- `--set` - top-level request field as name=json (repeatable)

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

idempotency, confirmation
