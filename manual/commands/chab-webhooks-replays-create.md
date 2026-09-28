# chab webhooks replays create

## Usage

```text
chab webhooks replays create [flags]
```

## Description

Create a webhook replay window.

Build the request from flags or provide an exact JSON object with --body or
--body-file. Flag mode requires --endpoint-id, at least one --event-type,
--created-after and --created-before. The server owns replay-window size,
retention, endpoint subscription and current authorization checks.

This action requires confirmation. In --no-prompt or non-interactive mode,
pass --yes after providing all request fields. Use --dry-run to preview the
request without credentials, confirmation or HTTP. Mutating requests send an
idempotency key; pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab webhooks deliveries list](chab-webhooks-deliveries-list.md)
- [chab webhooks endpoints show](chab-webhooks-endpoints-show.md)

## Examples

```text
  chab webhooks replays create --endpoint-id whe_example --event-type operation.succeeded --created-after START_TIME --created-before END_TIME --yes
  chab webhooks replays create --body-file replay.json --yes --json
  chab webhooks replays create --endpoint-id whe_example --event-type operation.succeeded --created-after START_TIME --created-before END_TIME --dry-run
  chab webhooks replays create --endpoint-id whe_example --event-type operation.succeeded --created-after START_TIME --created-before END_TIME --yes --jq .replay.id
  chab webhooks replays create --endpoint-id whe_example --event-type operation.succeeded --created-after START_TIME --created-before END_TIME --yes --template '{{.replay.id}}'
```

## Flags

- `--body` - JSON request body
- `--body-file` - read the JSON request body from a file, or - for stdin
- `--created-after` - replay window start date-time
- `--created-before` - replay window end date-time
- `--dry-run` - preview the request without resolving credentials or contacting the API
- `--endpoint-id` - webhook endpoint id
- `--event-type` - event type to replay (repeatable)
- `--idempotency-key` - explicit idempotency key; generated when omitted

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, metadata, confirmation
