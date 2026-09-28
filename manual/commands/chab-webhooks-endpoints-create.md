# chab webhooks endpoints create

## Usage

```text
chab webhooks endpoints create [flags]
```

## Description

Create a webhook endpoint and save its one-time signing secret.

Build the request from flags or provide an exact JSON object with --body or
--body-file. Flag mode requires --url and at least one --event-type. Optional
filters are --filter-project, --filter-operation-key, and --filter-family.
--enabled can be set explicitly; omitted lets the server apply its default.

The one-time signing secret is written only to --secret-out. The CLI reserves
that private path before the HTTP request and prints a safe receipt. Use
--dry-run to preview the request without reserving --secret-out, resolving
credentials or contacting the API. Mutating requests send an idempotency key;
pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default safe receipt, --plain, --json, --jq, and --template.

Related commands:
- [chab webhooks endpoints list](chab-webhooks-endpoints-list.md)
- [chab webhooks endpoints rotate-secret](chab-webhooks-endpoints-rotate-secret.md)

## Examples

```text
  chab webhooks endpoints create --url https://hook.example.test/chab-events --event-type operation.succeeded --filter-family search --secret-out webhook-secret.json
  chab webhooks endpoints create --body-file endpoint.json --secret-out webhook-secret.json --json
  chab webhooks endpoints create --url https://hook.example.test/chab-events --event-type operation.succeeded --dry-run
  chab webhooks endpoints create --url https://hook.example.test/chab-events --event-type operation.succeeded --secret-out webhook-secret.json --jq .secret_out
  chab webhooks endpoints create --url https://hook.example.test/chab-events --event-type operation.succeeded --secret-out webhook-secret.json --template '{{.secret_out}}'
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
- `--secret-out` - private path for the one-time signing secret
- `--url` - webhook HTTPS URL

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, metadata
