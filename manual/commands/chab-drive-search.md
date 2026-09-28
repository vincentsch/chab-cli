# chab drive search

## Usage

```text
chab drive search [flags]
```

## Description

Search connected drive item names.

Supply project_id, connection_id and query with flags, --input, or --set.
Search is a synchronous read and does not use --idempotency-key or --dry-run.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab drive items show](chab-drive-items-show.md)
- [chab drive items list](chab-drive-items-list.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab drive search --project-id 42 --connection-id 7 --query report
  chab drive search --input @drive-search.json --json --include-meta
  chab drive search --project-id 42 --connection-id 7 --query report --jq '.[0].item_id'
  chab drive search --project-id 42 --connection-id 7 --query report --template '{{range .}}{{.item_id}}{{"\n"}}{{end}}'
```

## Flags

- `--connection-id` - numeric drive connection id
- `--cursor` - opaque cursor
- `--input` - JSON request body, @path, or @- for stdin
- `--limit` - result limit
- `--project-id` - numeric Chab project id
- `--query` - literal name query
- `--set` - top-level request field as name=json (repeatable)

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata, preview
