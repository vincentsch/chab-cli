# chab tokens show

## Usage

```text
chab tokens show <token-public-id> [flags]
```

## Description

Show safe metadata for one API token.

The token public id is opaque and sent as one path segment. The plaintext token
secret is never available from this endpoint.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab tokens list](chab-tokens-list.md)
- [chab tokens update](chab-tokens-update.md)

## Examples

```text
  chab tokens show <token-public-id>
  chab tokens show <token-public-id> --json
  chab tokens show <token-public-id> --jq .token.policy_revision
  chab tokens show <token-public-id> --template '{{.token.policy_revision}}'
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
