# chab drive items list

## Usage

```text
chab drive items list [flags]
```

## Description

List drive items at the selected root or under one parent folder.

Pagination: --limit caps the total items fetched, --cursor starts at an opaque
API cursor, --page-size sets the per-request page size, and --all follows every
cursor page.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab drive items show](chab-drive-items-show.md)
- [chab drive search](chab-drive-search.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab drive items list --project-id 42 --connection-id 7
  chab drive items list --project-id 42 --connection-id 7 --parent-id dri_123 --json
  chab drive items list --project-id 42 --connection-id 7 --jq '.[0].item_id'
  chab drive items list --project-id 42 --connection-id 7 --template '{{range .}}{{.item_id}}{{"\n"}}{{end}}'
```

## Flags

- `--all` - fetch every cursor page
- `--connection-id` - numeric connected provider connection id
- `--cursor` - opaque cursor returned by the API
- `--limit` - maximum total number of items to fetch
- `--page-size` - items per API request (1-100)
- `--parent-id` - drive folder item id
- `--project-id` - numeric Chab project id

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

pagination, metadata, preview
