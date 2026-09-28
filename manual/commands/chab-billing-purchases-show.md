# chab billing purchases show

## Usage

```text
chab billing purchases show <purchase-id> [flags]
```

## Description

Show one credit purchase by its opaque pur_... id.

Polling reads require billing read scope independently of the original purchase
mutation scope.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default human summary, --plain, --json, --jq, and --template.

Related commands:
- [chab billing purchases create](chab-billing-purchases-create.md)
- [chab billing purchases wait](chab-billing-purchases-wait.md)

## Examples

```text
  chab billing purchases show pur_example
  chab billing purchases show pur_example --json
  chab billing purchases show pur_example --jq .purchase.state
  chab billing purchases show pur_example --template '{{.purchase.id}}'
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
