# chab operations list

## Usage

```text
chab operations list [flags]
```

## Description

List remote operations for the authenticated team.

Filters are passed directly to the Chab operation history endpoint. Cursor
metadata can be exposed with --include-meta in machine-output modes.
With --json, --jq, or --template, --include-meta wraps the operation list
under data and safe response metadata under meta. Use --plain for tabular
operation output.

Related commands:
- [chab operations show](chab-operations-show.md)
- [chab operations actions list](chab-operations-actions-list.md)

## Examples

```text
  chab operations list
  chab operations list --status running --operation-key search.web --json
  chab operations list --json --include-meta
  chab operations list --jq '.[].id'
  chab operations list --template '{{range .}}{{.id}}{{"\n"}}{{end}}'
  chab operations list --plain
```

## Flags

- `--cursor` - cursor for the next page
- `--family` - filter by operation family
- `--limit` - page size requested from the API
- `--operation-key` - filter by operation key
- `--status` - filter by operation status

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
