# chab mail search

## Usage

```text
chab mail search [flags]
```

## Description

Search connected mail with Chab's closed JSON request shape.

Supply --query, --filters as JSON, --input, or --set values. Search is a
synchronous read and does not use --idempotency-key or --dry-run.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab mail threads show](chab-mail-threads-show.md)
- [chab mail folders](chab-mail-folders.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab mail search --project-id 42 --connection-id 7 --query quarterly
  chab mail search --input @mail-search.json --json --include-meta
  chab mail search --project-id 42 --connection-id 7 --query quarterly --jq '.[0].thread.thread_id'
  chab mail search --project-id 42 --connection-id 7 --query quarterly --template '{{range .}}{{.thread.thread_id}}{{"\n"}}{{end}}'
```

## Flags

- `--connection-id` - numeric mail connection id
- `--cursor` - opaque cursor
- `--filters` - filter object as JSON
- `--folder-id` - numeric folder id
- `--input` - JSON request body, @path, or @- for stdin
- `--limit` - result limit
- `--project-id` - numeric Chab project id
- `--query` - literal search query
- `--set` - top-level request field as name=json (repeatable)

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata, preview
