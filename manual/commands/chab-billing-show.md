# chab billing show

## Usage

```text
chab billing show [flags]
```

## Description

Show billing context for the active team API key.

The response includes the server-owned plan name and current auto-recharge
summary. Use a manually created team API key when the server rejects broader
billing scopes for browser/device credentials.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab billing auto-recharge show](chab-billing-auto-recharge-show.md)
- [chab billing packages](chab-billing-packages.md)

## Examples

```text
  chab billing show
  chab billing show --json
  chab billing show --json --include-meta
  chab billing show --jq .plan
  chab billing show --template '{{.plan}}'
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
