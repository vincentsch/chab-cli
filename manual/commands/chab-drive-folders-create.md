# chab drive folders create

## Usage

```text
chab drive folders create [flags]
```

## Description

Create a drive folder.

Supply name, conflict_mode, project_id and connection_id with flags, --input,
or --set. Live requests use an Idempotency-Key and the accepted-operation
lifecycle; use --idempotency-key to supply one or let the CLI generate it. Use
--dry-run for server validation without mutation, and --wait to wait for the
accepted operation to finish.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab drive items list](chab-drive-items-list.md)
- [chab operations wait](chab-operations-wait.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab drive folders create --project-id 42 --connection-id 7 --name Reports --conflict-mode fail
  chab drive folders create --input @drive-folder.json --dry-run --json
```

## Flags

- `--conflict-mode` - conflict mode: fail or create_copy
- `--connection-id` - numeric drive connection id
- `--dry-run` - ask the server to validate without mutating provider data
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--input` - JSON request body, @path, or @- for stdin
- `--name` - folder name
- `--parent-id` - target parent folder id
- `--project-id` - numeric Chab project id
- `--set` - top-level request field as name=json (repeatable)
- `--wait` - wait for an accepted operation to reach a terminal state

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, preview
