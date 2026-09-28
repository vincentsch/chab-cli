# chab webhooks deliveries show

## Usage

```text
chab webhooks deliveries show <delivery-id> [flags]
```

## Description

Show one webhook delivery attempt.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab webhooks deliveries replay](chab-webhooks-deliveries-replay.md)
- [chab operations show](chab-operations-show.md)

## Examples

```text
  chab webhooks deliveries show <delivery-id>
  chab webhooks deliveries show <delivery-id> --json
  chab webhooks deliveries show <delivery-id> --jq .delivery.status
  chab webhooks deliveries show <delivery-id> --template '{{.delivery.id}}'
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
