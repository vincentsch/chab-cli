# chab credits balance

## Usage

```text
chab credits balance [flags]
```

## Description

Show the available credit balance.

For a team key, this authenticated read is team-level. For a guest trial key,
spendable_balance is zero and free_credits.available is the promotional balance
that funds supported guest operations.

JSON output includes integer spendable_balance and debt, optional free_credits,
and expires as the API timestamp string or null.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged.

Output modes: default human detail, --plain labeled tab-separated fields,
--json stable JSON, and --jq/--template transforms over the documented JSON
value.

Related commands:
- [chab credits transactions](chab-credits-transactions.md)
- [chab health](chab-health.md)

## Examples

```text
  chab credits balance
  chab credits balance --json
  chab credits balance --json --include-meta
  chab credits balance --jq .balance
  chab credits balance --template '{{.balance}}'
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
