# chab drive items upload

## Usage

```text
chab drive items upload [flags]
```

## Description

Upload an available stored file or eligible artifact into drive.

Supply source, name, project_id and connection_id with flags, --input, or
--set. Live requests use an Idempotency-Key and the accepted-operation
lifecycle; use --idempotency-key to supply one or let the CLI generate it. Use
--dry-run for server validation without mutation, and --wait to wait for the
accepted operation to finish.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab files upload](chab-files-upload.md)
- [chab operations wait](chab-operations-wait.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab drive items upload --project-id 42 --connection-id 7 --source '{"type":"file_id","id":"fil_123"}' --name report.pdf --conflict-mode fail
  chab drive items upload --input @drive-upload.json --dry-run --json
```

## Flags

- `--conflict-mode` - conflict mode: fail or create_copy
- `--connection-id` - numeric drive connection id
- `--dry-run` - ask the server to validate without mutating provider data
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--input` - JSON request body, @path, or @- for stdin
- `--name` - drive item name
- `--parent-id` - target parent folder id
- `--project-id` - numeric Chab project id
- `--set` - top-level request field as name=json (repeatable)
- `--source` - source object, for example {"type":"file_id","id":"fil_123"}
- `--wait` - wait for an accepted operation to reach a terminal state

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, preview
