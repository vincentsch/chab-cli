# chab drive items move

## Usage

```text
chab drive items move <item-id> [flags]
```

## Description

Move or rename a drive item.

Supply target_parent_id, expected_revision, project_id and connection_id with
flags, --input, or --set. Live requests use an Idempotency-Key; use
--idempotency-key to supply one or let the CLI generate it. They require
confirmation through --yes or an interactive prompt because they change remote
parentage or names. In --no-prompt mode, --yes is required. Use --dry-run for
server validation without mutation, and --wait to wait for the accepted
operation to finish.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab drive items show](chab-drive-items-show.md)
- [chab operations wait](chab-operations-wait.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab drive items move dri_123 --project-id 42 --connection-id 7 --target-parent-id dri_456 --expected-revision rev_1 --yes
  chab drive items move dri_123 --input @drive-move.json --dry-run --json
```

## Flags

- `--connection-id` - numeric drive connection id
- `--dry-run` - ask the server to validate without mutating provider data
- `--expected-revision` - expected item revision
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--input` - JSON request body, @path, or @- for stdin
- `--name` - new drive item name
- `--project-id` - numeric Chab project id
- `--set` - top-level request field as name=json (repeatable)
- `--target-parent-id` - target parent folder id
- `--wait` - wait for an accepted operation to reach a terminal state

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, confirmation, preview
