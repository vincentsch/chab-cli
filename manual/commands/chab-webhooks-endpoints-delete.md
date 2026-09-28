# chab webhooks endpoints delete

## Usage

```text
chab webhooks endpoints delete <endpoint-id> [flags]
```

## Description

Delete a webhook endpoint.

Deleting disables future deliveries and prevents replay. This action requires
confirmation; in --no-prompt or non-interactive mode, pass --yes after
providing the endpoint id. Use --dry-run to preview the request without
credentials, confirmation or HTTP. Mutating requests send an idempotency key;
pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab webhooks endpoints list](chab-webhooks-endpoints-list.md)
- [chab webhooks deliveries list](chab-webhooks-deliveries-list.md)

## Examples

```text
  chab webhooks endpoints delete <endpoint-id> --yes
  chab webhooks endpoints delete <endpoint-id> --dry-run
  chab webhooks endpoints delete <endpoint-id> --yes --jq .deleted
  chab webhooks endpoints delete <endpoint-id> --yes --template '{{.id}}'
```

## Flags

- `--dry-run` - preview the request without resolving credentials or contacting the API
- `--idempotency-key` - explicit idempotency key; generated when omitted

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, metadata, confirmation
