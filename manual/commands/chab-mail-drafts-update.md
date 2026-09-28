# chab mail drafts update

## Usage

```text
chab mail drafts update <draft-id> [flags]
```

## Description

Update a Chab API-owned mail draft.

Supply expected_draft_revision and changed composition fields with flags,
--input, or --set. Live requests use an Idempotency-Key; use --idempotency-key
to supply one or let the CLI generate it. Use --dry-run for server validation
without mutation.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab mail drafts show](chab-mail-drafts-show.md)
- [chab mail drafts attachments add](chab-mail-drafts-attachments-add.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab mail drafts update 21 --project-id 42 --connection-id 7 --expected-draft-revision 2 --subject Updated
  chab mail drafts update 21 --input @draft-update.json --dry-run --json
```

## Flags

- `--bcc` - ordered Bcc recipients array as JSON
- `--body` - body object, for example {"format":"plain","content":"Hello"}
- `--cc` - ordered Cc recipients array as JSON
- `--connection-id` - numeric mail connection id
- `--dry-run` - ask the server to validate without mutating provider data
- `--expected-draft-revision` - expected draft revision
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--input` - JSON request body, @path, or @- for stdin
- `--persona-id` - numeric sender persona id
- `--project-id` - numeric Chab project id
- `--set` - top-level request field as name=json (repeatable)
- `--subject` - draft subject
- `--to` - ordered To recipients array as JSON

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, preview
