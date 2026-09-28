# chab tokens revoke

## Usage

```text
chab tokens revoke <token-public-id> [flags]
```

## Description

Revoke an API token by public id and expected policy revision.

Revocation is permanent and requires confirmation. Cross-token revocation
requires an exact management approval proof; self-revocation is still confirmed
locally. --yes confirms only the revocation prompt and never supplies the
required revision or approval proof.
In --no-prompt or non-interactive mode, pass --yes after providing all required
inputs.

Use --dry-run to preview the request without credentials, proof input,
confirmation or HTTP. Mutating requests send an idempotency key; pass
--idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab tokens approvals create](chab-tokens-approvals-create.md)
- [chab tokens approvals wait](chab-tokens-approvals-wait.md)

## Examples

```text
  chab tokens revoke <token-public-id> --revision 3 --approval-proof-file proof.json --yes
  chab tokens revoke <token-public-id> --revision 3 --yes --json
  chab tokens revoke <token-public-id> --revision 3 --dry-run
  chab tokens revoke <token-public-id> --revision 3 --yes --jq .token.revoked
  chab tokens revoke <token-public-id> --revision 3 --yes --template '{{.token.revoked}}'
```

## Flags

- `--approval-proof-file` - private approval proof JSON file
- `--approval-proof-prompt` - read approval proof from a hidden prompt
- `--dry-run` - preview the request without resolving credentials or contacting the API
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--revision` - expected token policy revision

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, metadata, confirmation
