# chab webhooks endpoints list

## Usage

```text
chab webhooks endpoints list [flags]
```

## Description

List webhook endpoints owned by the current token.

The list is cursor-paginated. Use --limit, --cursor, --page-size, or --all.
--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab webhooks endpoints show](chab-webhooks-endpoints-show.md)
- [chab webhooks deliveries list](chab-webhooks-deliveries-list.md)

## Examples

```text
  chab webhooks endpoints list
  chab webhooks endpoints list --limit 20 --json
  chab webhooks endpoints list --all --json --include-meta
  chab webhooks endpoints list --jq '.[].id'
  chab webhooks endpoints list --template '{{range .}}{{.id}}{{"\n"}}{{end}}'
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
