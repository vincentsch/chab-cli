# chab files resume

## Usage

```text
chab files resume <action-id> <path> [flags]
```

## Description

Resume a stored-file upload from the private action journal.

The same file content and effective metadata must be supplied so the CLI can
verify the semantic fingerprint before replaying the original Idempotency-Key.
Completed actions are reported without resubmitting bytes. Replaying an upload
requires confirmation through --yes or an interactive prompt; in --no-prompt
or non-interactive mode, use --yes.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab files upload](chab-files-upload.md)
- [chab operations actions show](chab-operations-actions-show.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab files resume act_123 ./contract.pdf --yes
  chab files resume act_123 ./contract.pdf --project-id 42 --retention-hours 72 --json
```

## Flags

- `--filename` - trusted filename used by the original upload
- `--max-bytes` - maximum upload bytes to stream
- `--project-id` - numeric Chab project id used by the original upload
- `--retention-hours` - retention window used by the original upload
- `--wait` - wait for scanner readiness after upload acceptance

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

confirmation, preview
