# chab billing auto-recharge show

## Usage

```text
chab billing auto-recharge show [flags]
```

## Description

Show auto-recharge settings and current period counters.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab billing auto-recharge update](chab-billing-auto-recharge-update.md)
- [chab billing show](chab-billing-show.md)

## Examples

```text
  chab billing auto-recharge show
  chab billing auto-recharge show --json
  chab billing auto-recharge show --jq .enabled
  chab billing auto-recharge show --template '{{.enabled}}'
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
