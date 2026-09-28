# chab usage

## Usage

```text
chab usage [flags]
```

## Description

Show team API usage visible to the active API key.

The server owns project access, token visibility, date-window limits and plan
policy. Optional filters map directly to the public API query: --start, --end,
--timezone, --bucket (day or hour), and --group-by (operation_key, family,
project, or token). Browser device login currently grants only read-oriented
management scopes; use a manually created team API key when the server rejects
broader scopes with invalid_scope or api_scope_missing.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab credits balance](chab-credits-balance.md)
- [chab billing show](chab-billing-show.md)

## Examples

```text
  chab usage
  chab usage --start START_TIME --end END_TIME
  chab usage --bucket hour --group-by family --json
  chab usage --json --include-meta
  chab usage --jq '.groups[].final_credits'
  chab usage --template '{{.total_credits}}'
```

## Flags

- `--bucket` - usage bucket: day or hour
- `--end` - usage range end as an API date-time
- `--group-by` - usage grouping: operation_key, family, project, or token
- `--start` - usage range start as an API date-time
- `--timezone` - IANA timezone for bucket boundaries

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
