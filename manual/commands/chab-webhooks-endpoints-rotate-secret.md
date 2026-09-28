# chab webhooks endpoints rotate-secret

## Usage

```text
chab webhooks endpoints rotate-secret <endpoint-id> [flags]
```

## Description

Rotate a webhook endpoint signing secret.

The new one-time secret is written only to --secret-out. The CLI reserves that
private path before the HTTP request and prints a safe receipt. This action
requires confirmation; --yes confirms only the rotation prompt.

Use --dry-run to preview the request without reserving --secret-out,
credentials, confirmation or HTTP. Automatic retries are disabled for the
secret-producing request. Mutating requests send an idempotency key; pass
--idempotency-key to provide your own.
In --no-prompt or non-interactive mode, pass --yes after providing the endpoint
id and --secret-out.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default safe receipt, --plain, --json, --jq, and --template.

Related commands:
- [chab webhooks endpoints show](chab-webhooks-endpoints-show.md)
- [chab webhooks endpoints update](chab-webhooks-endpoints-update.md)

## Examples

```text
  chab webhooks endpoints rotate-secret <endpoint-id> --secret-out webhook-secret.json --yes
  chab webhooks endpoints rotate-secret <endpoint-id> --secret-out webhook-secret.json --yes --json
  chab webhooks endpoints rotate-secret <endpoint-id> --dry-run
  chab webhooks endpoints rotate-secret <endpoint-id> --secret-out webhook-secret.json --yes --jq .secret_out
  chab webhooks endpoints rotate-secret <endpoint-id> --secret-out webhook-secret.json --yes --template '{{.secret_out}}'
```

## Flags

- `--dry-run` - preview the request without resolving credentials or contacting the API
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--secret-out` - private path for the one-time signing secret

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, metadata, confirmation
