# chab mail drafts discard

## Usage

```text
chab mail drafts discard <draft-id> [flags]
```

## Description

Discard a mail draft.

Supply project_id, connection_id, and expected_draft_revision with flags,
--input, or --set. Live requests use an Idempotency-Key; use --idempotency-key
to supply one or let the CLI generate it. Use --dry-run for server validation
without mutation.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab mail drafts show](chab-mail-drafts-show.md)
- [chab mail drafts update](chab-mail-drafts-update.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab mail drafts discard 21 --project-id 42 --connection-id 7 --expected-draft-revision 3 --idempotency-key draft-discard-001
  chab mail drafts discard 21 --input @draft-discard.json --dry-run --json
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
