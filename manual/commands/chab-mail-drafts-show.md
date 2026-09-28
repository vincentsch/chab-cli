# chab mail drafts show

## Usage

```text
chab mail drafts show <draft-id> [flags]
```

## Description

Show a Chab API-owned mail draft.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab mail drafts update](chab-mail-drafts-update.md)
- [chab mail drafts send](chab-mail-drafts-send.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab mail drafts show 21 --project-id 42 --connection-id 7
  chab mail drafts show 21 --project-id 42 --connection-id 7 --json
  chab mail drafts show 21 --project-id 42 --connection-id 7 --jq .draft_id
  chab mail drafts show 21 --project-id 42 --connection-id 7 --template '{{.draft_id}}'
```

## Flags

- `--connection-id` - numeric connected provider connection id
- `--project-id` - numeric Chab project id

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata, preview
