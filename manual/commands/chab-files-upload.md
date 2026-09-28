# chab files upload

## Usage

```text
chab files upload <path> [flags]
```

## Description

Upload a stored file using multipart/form-data.

Uploads require an Idempotency-Key. The CLI creates a private local action
record before the live request and binds recovery to the file content plus the
effective filename, project and retention metadata, not the multipart boundary.
Use --idempotency-key to supply a caller-owned replay key; otherwise one is
generated. Uploads may consume paid storage and require confirmation through
--yes or an interactive prompt. In --no-prompt or non-interactive mode, use
--yes to acknowledge the upload. Use --wait to poll scanner readiness after the
server accepts the file.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab files wait](chab-files-wait.md)
- [chab convert file](chab-convert-file.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab files upload ./contract.pdf --project-id 42 --yes
  chab files upload ./notes.md --filename source.md --retention-hours 72 --wait --json
  chab files upload ./contract.pdf --idempotency-key upload-contract-001 --yes --plain
```

## Flags

- `--filename` - trusted filename sent to Chab
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--max-bytes` - maximum upload bytes to stream
- `--project-id` - numeric Chab project id to make the file project-visible
- `--retention-hours` - retention window in hours, 24 through 720
- `--wait` - wait for scanner readiness after upload acceptance

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

idempotency, confirmation, preview
