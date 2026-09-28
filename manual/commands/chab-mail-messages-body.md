# chab mail messages body

## Usage

```text
chab mail messages body <message-id> [flags]
```

## Description

Read one sanitized mail message body.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab mail threads show](chab-mail-threads-show.md)
- [chab mail attachments download](chab-mail-attachments-download.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab mail messages body 84 --project-id 42 --connection-id 7
  chab mail messages body 84 --project-id 42 --connection-id 7 --format markdown --json
  chab mail messages body 84 --project-id 42 --connection-id 7 --jq .content
  chab mail messages body 84 --project-id 42 --connection-id 7 --template '{{.content}}'
```

## Flags

- `--connection-id` - numeric connected provider connection id
- `--format` - body format: plain, html, or markdown
- `--project-id` - numeric Chab project id

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata, preview
