# chab webhooks deliveries list

## Usage

```text
chab webhooks deliveries list [flags]
```

## Description

List webhook delivery attempts for current-token endpoints.

Optional filters are --endpoint-id and --status. The list is cursor-paginated;
use --limit, --cursor, --page-size, or --all. --include-meta adds safe
transport context under meta when --json, --jq, or --template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab webhooks deliveries show](chab-webhooks-deliveries-show.md)
- [chab webhooks deliveries replay](chab-webhooks-deliveries-replay.md)

## Examples

```text
  chab webhooks deliveries list
  chab webhooks deliveries list --endpoint-id whe_example --status failed --json
  chab webhooks deliveries list --all --json --include-meta
  chab webhooks deliveries list --jq '.[].id'
  chab webhooks deliveries list --template '{{range .}}{{.id}}{{"\n"}}{{end}}'
```

## Flags

- `--all` - fetch every cursor page
- `--cursor` - opaque cursor returned by the API
- `--endpoint-id` - filter by endpoint id
- `--limit` - maximum total number of items to fetch
- `--page-size` - items per API request (1-100)
- `--status` - filter by delivery status

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

pagination, metadata
