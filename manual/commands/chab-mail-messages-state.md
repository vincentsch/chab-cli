# chab mail messages state

## Usage

```text
chab mail messages state <message-id> [flags]
```

## Description

Update mail message state with Chab's documented actions.

Supply project_id, connection_id, expected_state_revision, action, and any
required folder fields with flags, --input, or --set. Live requests use an
Idempotency-Key; use --idempotency-key to supply one or let the CLI generate
it. Use --dry-run for server validation without mutation.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab mail messages body](chab-mail-messages-body.md)
- [chab mail threads list](chab-mail-threads-list.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab mail messages state 84 --project-id 42 --connection-id 7 --expected-state-revision 4 --action mark_read
  chab mail messages state 84 --input @message-state.json --dry-run --json
```

## Flags

- `--action` - state action
- `--connection-id` - numeric mail connection id
- `--dry-run` - ask the server to validate without mutating provider data
- `--expected-state-revision` - expected message state revision
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--input` - JSON request body, @path, or @- for stdin
- `--project-id` - numeric Chab project id
- `--set` - top-level request field as name=json (repeatable)
- `--source-folder-id` - source folder id for archive, trash, or move
- `--target-folder-id` - target folder id for move

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, preview
