# chab drive permissions update

## Usage

```text
chab drive permissions update <item-id> [flags]
```

## Description

Update direct drive item permissions.

Supply action, principal, role when required, expected_permission_revision,
project_id and connection_id with flags, --input, or --set. Live requests use
an Idempotency-Key; use --idempotency-key to supply one or let the CLI
generate it. They require confirmation through --yes or an interactive prompt
because they change sharing. In --no-prompt mode, --yes is required. Use
--dry-run for server validation without mutation, and --wait to wait for the
accepted operation to finish.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab drive items show](chab-drive-items-show.md)
- [chab operations wait](chab-operations-wait.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab drive permissions update dri_123 --project-id 42 --connection-id 7 --action update --principal '{"type":"user","email":"ada@example.com"}' --role viewer --expected-permission-revision perm_1 --yes
  chab drive permissions update dri_123 --input @drive-permission.json --dry-run --json
```

## Flags

- `--action` - permission action: grant, update, or revoke
- `--connection-id` - numeric drive connection id
- `--dry-run` - ask the server to validate without mutating provider data
- `--expected-permission-revision` - expected permission revision
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--input` - JSON request body, @path, or @- for stdin
- `--principal` - principal object, for example {"type":"user","email":"ada@example.com"}
- `--project-id` - numeric Chab project id
- `--role` - role for grant or update: viewer, commenter, or editor
- `--set` - top-level request field as name=json (repeatable)
- `--wait` - wait for an accepted operation to reach a terminal state

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, confirmation, preview
