# chab mail drafts attachments add

## Usage

```text
chab mail drafts attachments add <draft-id> [flags]
```

## Description

Add one eligible operation artifact to a draft.

Live requests use an Idempotency-Key; use --idempotency-key to supply one or
let the CLI generate it. Use --dry-run for server validation without mutation.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab operations artifact](chab-operations-artifact.md)
- [chab mail drafts attachments remove](chab-mail-drafts-attachments-remove.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab mail drafts attachments add 21 --project-id 42 --connection-id 7 --expected-draft-revision 2 --artifact-id art_123
  chab mail drafts attachments add 21 --input @attachment.json --dry-run --json
```

## Flags

- `--artifact-id` - eligible operation artifact id
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
