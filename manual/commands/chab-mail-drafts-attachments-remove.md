# chab mail drafts attachments remove

## Usage

```text
chab mail drafts attachments remove <draft-id> <attachment-id> [flags]
```

## Description

Remove one staged draft attachment.

Live requests use an Idempotency-Key; use --idempotency-key to supply one or
let the CLI generate it. Use --dry-run for server validation without mutation.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab mail drafts attachments add](chab-mail-drafts-attachments-add.md)
- [chab mail drafts show](chab-mail-drafts-show.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab mail drafts attachments remove 21 staged_123 --project-id 42 --connection-id 7 --expected-draft-revision 3
  chab mail drafts attachments remove 21 staged_123 --dry-run --json
```

## Flags

- `--connection-id` - numeric mail connection id
- `--dry-run` - ask the server to validate without mutating provider data
- `--expected-draft-revision` - expected draft revision
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--input` - JSON request body, @path, or @- for stdin
- `--project-id` - numeric Chab project id
- `--set` - top-level request field as name=json (repeatable)

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, preview
