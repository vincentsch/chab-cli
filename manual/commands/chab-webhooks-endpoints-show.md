# chab webhooks endpoints show

## Usage

```text
chab webhooks endpoints show <endpoint-id> [flags]
```

## Description

Show one webhook endpoint without its signing secret.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab webhooks endpoints update](chab-webhooks-endpoints-update.md)
- [chab webhooks endpoints rotate-secret](chab-webhooks-endpoints-rotate-secret.md)

## Examples

```text
  chab webhooks endpoints show <endpoint-id>
  chab webhooks endpoints show <endpoint-id> --json
  chab webhooks endpoints show <endpoint-id> --jq .endpoint.enabled
  chab webhooks endpoints show <endpoint-id> --template '{{.endpoint.id}}'
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
