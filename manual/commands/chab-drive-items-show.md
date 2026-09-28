# chab drive items show

## Usage

```text
chab drive items show <item-id> [flags]
```

## Description

Show drive item metadata.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab drive items download](chab-drive-items-download.md)
- [chab drive items update](chab-drive-items-update.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab drive items show dri_123 --project-id 42 --connection-id 7
  chab drive items show dri_123 --project-id 42 --connection-id 7 --json
  chab drive items show dri_123 --project-id 42 --connection-id 7 --jq .item_id
  chab drive items show dri_123 --project-id 42 --connection-id 7 --template '{{.item_id}}'
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
