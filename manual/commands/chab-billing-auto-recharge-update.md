# chab billing auto-recharge update

## Usage

```text
chab billing auto-recharge update [flags]
```

## Description

Update auto-recharge settings after management approval.

Build the request with flags or provide the exact JSON body via --body or
--body-file. Flag mode requires --enabled and sends only fields explicitly set:
--package-id, --package-id-null, --threshold, --threshold-null, --daily-limit,
--daily-limit-null, --max-per-period, and --max-per-period-null.

Live updates require --approval-proof-file or --approval-proof-prompt. The proof
is sent only in the X-Chab-Management-Approval header and never printed. Use
--dry-run to preview the request without credentials, proof input, confirmation
or HTTP. Mutating requests send an idempotency key; pass --idempotency-key to
provide your own.

This action requires confirmation. In --no-prompt or non-interactive mode,
pass --yes after providing all required request fields and approval proof.
Browser device login may not grant billing write scope; use a manually created
team API key when the server rejects broader scopes.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab tokens approvals create](chab-tokens-approvals-create.md)
- [chab tokens approvals wait](chab-tokens-approvals-wait.md)

## Examples

```text
  chab billing auto-recharge update --enabled --package-id credits_1000 --threshold 100 --daily-limit 2000 --max-per-period 5 --approval-proof-file proof.json --yes
  chab billing auto-recharge update --body-file auto-recharge.json --approval-proof-file proof.json --yes --json
  chab billing auto-recharge update --enabled=false --package-id-null --threshold-null --daily-limit-null --max-per-period-null --dry-run
  chab billing auto-recharge update --enabled=false --approval-proof-file proof.json --yes --jq .enabled
  chab billing auto-recharge update --enabled=false --approval-proof-file proof.json --yes --template '{{.enabled}}'
```

## Flags

- `--approval-proof-file` - private approval proof JSON file
- `--approval-proof-prompt` - read approval proof from a hidden prompt
- `--body` - JSON request body
- `--body-file` - read the JSON request body from a file, or - for stdin
- `--daily-limit` - daily recharge limit
- `--daily-limit-null` - send daily_limit as null
- `--dry-run` - preview the request without resolving credentials or contacting the API
- `--enabled` - enable or disable auto-recharge
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--max-per-period` - maximum attempts per billing period
- `--max-per-period-null` - send max_per_period as null
- `--package-id` - credit package id
- `--package-id-null` - send package_id as null
- `--threshold` - credit threshold
- `--threshold-null` - send threshold as null

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, metadata, confirmation
