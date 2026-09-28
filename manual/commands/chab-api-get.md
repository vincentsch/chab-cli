# chab api get

## Usage

```text
chab api get <path> [flags]
```

## Description

Send a GET request to an API path using the active profile and credential.

<path> is API-relative and is joined below the resolved API base. Give a path
like /projects or projects. A bare first segment containing a colon is rejected
as ambiguous URI syntax; a colon after an explicit leading slash or in a later
segment remains opaque path data. Absolute and scheme-relative URLs, query or
fragment markers, trailing slashes, and empty segments fail locally. Put query
parameters in --query key=value (repeatable); values may be empty or contain
commas and equals signs, and repeated values retain their order.

By default the decoded "data" value of the API success envelope is printed. Use
--raw to print the full success envelope, including data, meta, request_id, and
unknown envelope keys.

Without pagination flags, GET performs exactly one request and does not inject
cursor or limit query parameters. Use --all to follow every cursor page,
--limit to cap merged rows, --cursor to start from an API cursor, or
--page-size to set the API request limit. Pagination flags cannot be combined
with --query cursor=... or --query limit=.... --raw is allowed with --cursor
or --page-size, but --raw cannot be used with --all or --limit because those
modes merge data rows. --all=false leaves pagination inactive. When the first
--all or --limit response is not a cursor-paginated array, its single data
value is rendered as-is. Later response-shape or pagination drift is a protocol
error and produces no partial output.

Mark a request value as sensitive with --secret-field <name> (repeatable). The
named query value is shown as [REDACTED] in --debug output and rendered errors;
the flag does not add a request value.

JSON output is the selected machine value as stable JSON. Default human and
--plain output print scalar values without JSON quotes; objects, arrays, and
null print as JSON because the raw shape is not known ahead of time.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data. Decoded GET also
places safe non-pagination envelope context under meta.api_meta; --raw keeps
the selected full envelope under data without duplicating that context.
Output without --include-meta remains unchanged.

Output modes: default human, --plain, --json stable JSON, and --jq/--template
transforms over the selected JSON value.

Related commands:
- [chab api](chab-api.md)
- [chab login](chab-login.md)

## Examples

```text
  chab api get /me
  chab api get /me --json
  chab api get projects --query status=active
  chab api get projects --query status=active --raw --json
  chab api get /projects --all --json
  chab api get /projects --all --json --include-meta
  chab api get /credits --jq .spendable_balance
  chab api get /projects --template '{{range .}}{{.id}}{{end}}'
  chab api get /me --secret-field token --query token=<token> --debug
```

## Flags

- `--all` - fetch every cursor page
- `--cursor` - opaque cursor returned by the API
- `--limit` - maximum total number of items to fetch
- `--page-size` - items per API request (1-100)
- `--query` - query parameter as key=value (repeatable)
- `--raw` - print the full API success envelope instead of only data
- `--secret-field` - request field name whose value must be redacted in debug output and errors (repeatable)

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

pagination, raw-envelope, metadata
