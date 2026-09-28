# chab project list

## Usage

```text
chab project list [flags]
```

## Description

List projects visible to the active team API key.

Pagination: --limit caps the total items fetched, --cursor starts at an opaque
API cursor, --page-size sets the per-request page size, and --all follows every
cursor page. With no list-size flag, the server's first-page default is used.

JSON output is an array of project objects with fields id, name, description,
url, status, timezone, language, limit, automate, created_at, updated_at.
Default JSON omits request and pagination metadata.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged.

Output modes: default human table, --plain tab-separated rows, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
- [chab project show](chab-project-show.md)
- [chab credits](chab-credits.md)

## Examples

```text
  chab project list
  chab project list --limit 50
  chab project list --cursor <cursor> --page-size 50
  chab project list --json
  chab project list --all --json --include-meta
  chab project list --jq '.[].id'
  chab project list --template '{{range .}}{{.id}}{{"\n"}}{{end}}'
```

## Flags

- `--all` - fetch every cursor page
- `--cursor` - opaque cursor returned by the API
- `--limit` - maximum total number of items to fetch
- `--page-size` - items per API request (1-100)

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

pagination, metadata
