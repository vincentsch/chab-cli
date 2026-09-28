# chab mail threads show

## Usage

```text
chab mail threads show <thread-id> [flags]
```

## Description

Show a mail thread and its draft-free messages.

Pagination: --limit caps the total messages fetched, --cursor starts at an
opaque API cursor, --page-size sets the per-request page size, and --all
follows every cursor page.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab mail threads list](chab-mail-threads-list.md)
- [chab mail messages body](chab-mail-messages-body.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab mail threads show 88 --project-id 42 --connection-id 7
  chab mail threads show 88 --project-id 42 --connection-id 7 --json --include-meta
  chab mail threads show 88 --project-id 42 --connection-id 7 --jq .thread.thread_id
  chab mail threads show 88 --project-id 42 --connection-id 7 --template '{{.thread.thread_id}}'
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
