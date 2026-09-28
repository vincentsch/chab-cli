# chab operations resume

## Usage

```text
chab operations resume <action-id> [flags]
```

## Description

Resume a local action from the private journal.

Unknown outcomes that have no accepted remote ID require the original request
input so the CLI can verify the same deterministic request bytes before it
replays with the stored idempotency key. Actions with an accepted operation ID
or a completed local receipt are reported without resubmission.
An existing prepared receipt has no persisted response and cannot be replayed:
the earlier request may have been denied while local persistence failed.
Stored-file upload and deletion actions continue through the stored-file
resource lifecycle and may return a file receipt rather than an operation
receipt.
Use --json for the stable action and recovery object and --plain for the
resumed operation id or action id.

Related commands:
- [chab operations actions show](chab-operations-actions-show.md)
- [chab operations wait](chab-operations-wait.md)

## Examples

```text
  chab operations resume act_123 --input @request.json
  chab operations resume act_123 --json
  chab operations resume act_123 --input @request.json --json
  chab operations resume act_123 --input @request.json --plain
```

## Flags

- `--input` - JSON request body, @path, or @- for stdin
- `--set` - top-level request field as name=json (repeatable)

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.
