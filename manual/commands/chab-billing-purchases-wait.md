# chab billing purchases wait

## Usage

```text
chab billing purchases wait <purchase-id> [flags]
```

## Description

Wait for a purchase to leave pending or pending_reconciliation.

The command follows the supplied purchase id with GET
/v1/billing/purchases/{purchase_id}. It does not create another purchase.
Retry-After is honored when present; otherwise next_check_at is used.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default human summary, --plain, --json, --jq, and --template.

Related commands:
- [chab billing purchases show](chab-billing-purchases-show.md)
- [chab credits balance](chab-credits-balance.md)

## Examples

```text
  chab billing purchases wait pur_example
  chab billing purchases wait pur_example --json
  chab billing purchases wait pur_example --jq .purchase.state
  chab billing purchases wait pur_example --template '{{.purchase.id}}'
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
