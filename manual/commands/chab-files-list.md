# chab files list

## Usage

```text
chab files list [flags]
```

## Description

List stored files visible to the active API key.

Pagination: --limit caps the total items fetched, --cursor starts at an opaque
API cursor, --page-size sets the per-request page size, and --all follows every
cursor page.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab files show](chab-files-show.md)
- [chab files upload](chab-files-upload.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab files list
  chab files list --project-id 42 --state available
  chab files list --all --json --include-meta
  chab files list --jq '.[0].id'
  chab files list --template '{{range .}}{{.id}}{{"\n"}}{{end}}'
```

## Flags

- `--all` - fetch every cursor page
- `--created-after` - include files created at or after this API date-time
- `--created-before` - include files created at or before this API date-time
- `--cursor` - opaque cursor returned by the API
- `--limit` - maximum total number of items to fetch
- `--page-size` - items per API request (1-100)
- `--project-id` - numeric Chab project id
- `--state` - file state filter
- `--visibility` - visibility filter: creator or project

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

pagination, metadata, preview
