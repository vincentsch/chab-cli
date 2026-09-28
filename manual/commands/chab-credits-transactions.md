# chab credits transactions

## Usage

```text
chab credits transactions [flags]
```

## Description

List credit transactions for the authenticated team.

Credits are team-level: the transaction history is not narrowed by a
selected-project key scope.

Pagination uses Chab cursor metadata. --cursor starts from a server-provided
cursor, --page-size sets the API request limit, --limit caps total rows, and
--all follows next_cursor until the API reports no more pages.

JSON output is an array of transaction objects with fields id, occurred_at,
kind, amount, resulting_spendable_balance, expires_at, operation_id,
purchase_id, and description. description is localized display text and must
not be parsed for behavior. amount is a signed integer credit unit. Default
JSON omits request and pagination metadata.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged.

Output modes: default human table, --plain tab-separated rows, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
- [chab credits balance](chab-credits-balance.md)
- [chab health](chab-health.md)

## Examples

```text
  chab credits transactions
  chab credits transactions --limit 250
  chab credits transactions --cursor eyJpZCI6MX0 --page-size 25
  chab credits transactions --json
  chab credits transactions --include-meta --jq .meta.cursor.next_cursor
  chab credits transactions --jq '.[].amount'
  chab credits transactions --template '{{range .}}{{.id}} {{.amount}}{{"\n"}}{{end}}'
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
