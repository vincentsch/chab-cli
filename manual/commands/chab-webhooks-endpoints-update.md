# chab webhooks endpoints update

## Usage

```text
chab webhooks endpoints update <endpoint-id> [flags]
```

## Description

Update a webhook endpoint.

Build the request from flags or provide an exact JSON object with --body or
--body-file. At least one field must change. The server owns URL safety,
enabled endpoint quotas, filter limits and current authorization checks.

Use --dry-run to preview the request without credentials or HTTP. Mutating
requests send an idempotency key; pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab webhooks endpoints show](chab-webhooks-endpoints-show.md)
- [chab webhooks deliveries list](chab-webhooks-deliveries-list.md)

## Examples

```text
  chab webhooks endpoints update <endpoint-id> --event-type operation.succeeded --event-type operation.failed
  chab webhooks endpoints update <endpoint-id> --enabled=false --json
  chab webhooks endpoints update <endpoint-id> --body-file endpoint-update.json
  chab webhooks endpoints update <endpoint-id> --enabled=false --jq .endpoint.enabled
  chab webhooks endpoints update <endpoint-id> --enabled=false --template '{{.endpoint.id}}'
```

## Flags

- `--body` - JSON request body
- `--body-file` - read the JSON request body from a file, or - for stdin
- `--dry-run` - preview the request without resolving credentials or contacting the API
- `--enabled` - enable or disable the endpoint
- `--event-type` - event type subscription (repeatable)
- `--filter-family` - operation family filter (repeatable)
- `--filter-operation-key` - operation-key filter (repeatable)
- `--filter-project` - project filter id (repeatable)
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--url` - webhook HTTPS URL

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, metadata
