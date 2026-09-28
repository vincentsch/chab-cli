# chab mail folders

## Usage

```text
chab mail folders [flags]
```

## Description

List mail folders.

Pagination: --limit caps the total items fetched, --cursor starts at an opaque
API cursor, --page-size sets the per-request page size, and --all follows every
cursor page.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab mail threads](chab-mail-threads.md)
- [chab mail connections](chab-mail-connections.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab mail folders --project-id 42 --connection-id 7
  chab mail folders --project-id 42 --connection-id 7 --json
  chab mail folders --project-id 42 --connection-id 7 --jq '.[0].folder_id'
  chab mail folders --project-id 42 --connection-id 7 --template '{{range .}}{{.folder_id}}{{"\n"}}{{end}}'
```

## Flags

- `--all` - fetch every cursor page
- `--connection-id` - numeric connected provider connection id
- `--cursor` - opaque cursor returned by the API
- `--limit` - maximum total number of items to fetch
- `--page-size` - items per API request (1-100)
- `--project-id` - numeric Chab project id

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

pagination, metadata, preview
