# chab billing purchases create

## Usage

```text
chab billing purchases create [flags]
```

## Description

Create a credit package purchase after management approval.

Provide --package-id and a private approval proof from chab tokens approvals
wait. The proof is sent only in X-Chab-Management-Approval. The command records
one local action and one idempotency key before the HTTP request; pass
--idempotency-key to provide your own. Rerunning with the same key reuses the
purchase resource instead of creating another charge. Pass --wait to poll
pending and pending_reconciliation until fulfilled or failed.

This action requires confirmation. In --no-prompt or non-interactive mode,
pass --yes after providing all required inputs and proof. Browser device login
may not grant billing write scope; use a manually created team API key when
needed.

Use --dry-run to preview the request without credentials, proof input,
confirmation or HTTP.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default human summary, --plain, --json, --jq, and --template.

Related commands:
- [chab billing purchases show](chab-billing-purchases-show.md)
- [chab billing purchases wait](chab-billing-purchases-wait.md)

## Examples

```text
  chab billing purchases create --package-id credits_1000 --approval-proof-file proof.json --yes
  chab billing purchases create --package-id credits_1000 --approval-proof-file proof.json --yes --wait --json
  chab billing purchases create --package-id credits_1000 --dry-run
  chab billing purchases create --package-id credits_1000 --approval-proof-file proof.json --yes --jq .purchase.id
  chab billing purchases create --package-id credits_1000 --approval-proof-file proof.json --yes --template '{{.purchase.id}}'
```

## Flags

- `--approval-proof-file` - private approval proof JSON file
- `--approval-proof-prompt` - read approval proof from a hidden prompt
- `--dry-run` - preview the request without resolving credentials or contacting the API
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--package-id` - credit package id
- `--wait` - poll the returned purchase until terminal

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, metadata, confirmation
