# chab mail threads list

## Usage

```text
chab mail threads list [flags]
```

## Description

List mail threads in the persisted Inbox or one selected folder.

Pagination: --limit caps the total items fetched, --cursor starts at an opaque
API cursor, --page-size sets the per-request page size, and --all follows every
cursor page.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab mail threads show](chab-mail-threads-show.md)
- [chab mail search](chab-mail-search.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab mail threads list --project-id 42 --connection-id 7
  chab mail threads list --project-id 42 --connection-id 7 --folder-id 3 --json
  chab mail threads list --project-id 42 --connection-id 7 --jq '.[0].thread_id'
  chab mail threads list --project-id 42 --connection-id 7 --template '{{range .}}{{.thread_id}}{{"\n"}}{{end}}'
```

## Flags

- `--all` - fetch every cursor page
- `--connection-id` - numeric connected provider connection id
- `--cursor` - opaque cursor returned by the API
- `--folder-id` - numeric folder id
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
