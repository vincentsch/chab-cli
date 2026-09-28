# chab tokens approvals create

## Usage

```text
chab tokens approvals create [flags]
```

## Description

Create a management approval challenge.

Provide --action and --mutation. Use --target-token for token-targeted actions
such as tokens.update and tokens.revoke; when omitted, the current team is used
for tokens.create, billing.purchases.create, and billing.auto_recharge.update.
For token-targeted actions, --revision is sent as expected_policy_revision
outside mutation and must match the bound mutation request later.

The response includes a product-web verification URL resolved from the profile
base URL, not the API base URL. Complete that web approval, then run
chab tokens approvals wait <approval-id> --proof-out <path>.

Mutating requests send an idempotency key; pass --idempotency-key to provide
your own. Browser device login may not grant token or billing write scopes; use
a manually created team API key when needed.

Use --dry-run to preview the request without resolving credentials or
contacting the API. --include-meta adds safe transport context under meta when
--json, --jq, or --template is selected. It cannot be combined with --dry-run.

Output modes: default human detail, --plain, --json, --jq, and --template.

Related commands:
- [chab tokens approvals wait](chab-tokens-approvals-wait.md)
- [chab tokens revoke](chab-tokens-revoke.md)

## Examples

```text
  chab tokens approvals create --action tokens.revoke --target-token <token-public-id> --revision 3 --mutation @empty.json
  chab tokens approvals create --action billing.purchases.create --mutation '{"package_id":"credits_1000"}' --json
  chab tokens approvals create --action tokens.create --mutation @token.json --idempotency-key <key>
  chab tokens approvals create --action tokens.create --mutation @token.json --jq .approval_id
  chab tokens approvals create --action tokens.create --mutation @token.json --template '{{.approval_id}}'
```

## Flags

- `--action` - protected action, such as tokens.revoke
- `--dry-run` - preview the request without resolving credentials or contacting the API
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--mutation` - mutation JSON, @path, or @-
- `--revision` - expected token policy revision for token-targeted actions
- `--target-token` - target token public id for token-targeted actions

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, metadata
