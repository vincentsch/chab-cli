# chab mail drafts create

## Usage

```text
chab mail drafts create [flags]
```

## Description

Create a Chab API-owned mail draft.

Supply composition with convenience flags, --input, or --set. Recipient arrays
and body are exact JSON values. Live requests use an Idempotency-Key; use
--idempotency-key to supply one or let the CLI generate it. Use --dry-run for
server validation without mutation.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab mail drafts show](chab-mail-drafts-show.md)
- [chab mail drafts send](chab-mail-drafts-send.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab mail drafts create --project-id 42 --connection-id 7 --mode new --to '[{"address":"ada@example.com"}]' --subject Update --body '{"format":"plain","content":"Hello"}'
  chab mail drafts create --input @draft.json --dry-run --json
```

## Flags

- `--bcc` - ordered Bcc recipients array as JSON
- `--body` - body object, for example {"format":"plain","content":"Hello"}
- `--cc` - ordered Cc recipients array as JSON
- `--connection-id` - numeric mail connection id
- `--dry-run` - ask the server to validate without mutating provider data
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--input` - JSON request body, @path, or @- for stdin
- `--mode` - draft mode: new or reply
- `--persona-id` - numeric sender persona id
- `--project-id` - numeric Chab project id
- `--reply-to-message-id` - message id for reply drafts
- `--set` - top-level request field as name=json (repeatable)
- `--subject` - draft subject
- `--to` - ordered To recipients array as JSON

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, preview
