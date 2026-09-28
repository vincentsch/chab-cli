# chab billing reconciliation

## Usage

```text
chab billing reconciliation [flags]
```

## Description

Show the latest billing reconciliation report visible to the active API key.

The server returns either the latest clean or differences-found report, or a
not_yet_available state. The CLI does not run reconciliation work locally.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab billing purchases show](chab-billing-purchases-show.md)
- [chab credits transactions](chab-credits-transactions.md)

## Examples

```text
  chab billing reconciliation
  chab billing reconciliation --json
  chab billing reconciliation --jq .state
  chab billing reconciliation --template '{{.state}}'
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
