# chab drive items update

## Usage

```text
chab drive items update <item-id> [flags]
```

## Description

Replace one drive file's bytes with an available stored file or eligible artifact.

Supply source, expected_revision, project_id and connection_id with flags,
--input, or --set. Live requests use an Idempotency-Key; use
--idempotency-key to supply one or let the CLI generate it. They require
confirmation through --yes or an interactive prompt because they change remote
file content. In --no-prompt mode, --yes is required. Use --dry-run for server
validation without mutation, and --wait to wait for the accepted operation to
finish.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab drive items show](chab-drive-items-show.md)
- [chab operations wait](chab-operations-wait.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab drive items update dri_123 --project-id 42 --connection-id 7 --source '{"type":"file_id","id":"fil_123"}' --expected-revision rev_1 --yes
  chab drive items update dri_123 --input @drive-update.json --dry-run --json
```

## Flags

- `--connection-id` - numeric drive connection id
- `--dry-run` - ask the server to validate without mutating provider data
- `--expected-revision` - expected item revision
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--input` - JSON request body, @path, or @- for stdin
- `--project-id` - numeric Chab project id
- `--set` - top-level request field as name=json (repeatable)
- `--source` - source object, for example {"type":"file_id","id":"fil_123"}
- `--wait` - wait for an accepted operation to reach a terminal state

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, confirmation, preview
