# chab webhooks deliveries replay

## Usage

```text
chab webhooks deliveries replay <delivery-id> [flags]
```

## Description

Replay one webhook delivery after current server authorization checks.

This action requires confirmation; in --no-prompt or non-interactive mode,
pass --yes after providing the delivery id. Use --dry-run to preview the
request without credentials, confirmation or HTTP. Mutating requests send an
idempotency key; pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab webhooks deliveries show](chab-webhooks-deliveries-show.md)
- [chab webhooks replays create](chab-webhooks-replays-create.md)

## Examples

```text
  chab webhooks deliveries replay <delivery-id> --yes
  chab webhooks deliveries replay <delivery-id> --yes --json
  chab webhooks deliveries replay <delivery-id> --dry-run
  chab webhooks deliveries replay <delivery-id> --yes --jq .replay.id
  chab webhooks deliveries replay <delivery-id> --yes --template '{{.replay.id}}'
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
