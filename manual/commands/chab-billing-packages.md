# chab billing packages

## Usage

```text
chab billing packages [flags]
```

## Description

List credit packages configured by the server.

The server decides which packages are purchaseable by the team. This command is
read-only and does not create a purchase.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab billing purchases create](chab-billing-purchases-create.md)
- [chab credits balance](chab-credits-balance.md)

## Examples

```text
  chab billing packages
  chab billing packages --json
  chab billing packages --jq '.packages[].id'
  chab billing packages --template '{{range .packages}}{{.id}}{{"\n"}}{{end}}'
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
