# chab files delete

## Usage

```text
chab files delete <file-id> [flags]
```

## Description

Delete a stored file.

Deletion requires confirmation through --yes or an interactive prompt and sends
one Idempotency-Key for the logical action. In --no-prompt or non-interactive
mode, use --yes. Use --idempotency-key to supply a
caller-owned replay key; otherwise one is generated. If the server returns
deletion_pending, --wait polls the file resource until it disappears or returns
a terminal deletion receipt; the command does not claim proof of physical byte
purge.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab files list](chab-files-list.md)
- [chab operations actions show](chab-operations-actions-show.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab files delete fil_123 --yes
  chab files delete fil_123 --idempotency-key delete-fil-123 --yes --json
  chab files delete fil_123 --yes --wait
```

## Flags

- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--wait` - wait for deletion_pending to become a terminal lifecycle result

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

idempotency, confirmation, preview
