# chab tokens list

## Usage

```text
chab tokens list [flags]
```

## Description

List team API tokens visible to the active API key.

The list is cursor-paginated. Use --limit to cap returned items, --cursor to
resume from an API cursor, --page-size for per-request size, or --all to fetch
every page.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab tokens show](chab-tokens-show.md)
- [chab whoami](chab-whoami.md)

## Examples

```text
  chab tokens list
  chab tokens list --limit 20 --json
  chab tokens list --all --json --include-meta
  chab tokens list --jq '.[].token_public_id'
  chab tokens list --template '{{range .}}{{.token_public_id}}{{"\n"}}{{end}}'
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
