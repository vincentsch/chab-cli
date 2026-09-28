# chab tokens create

## Usage

```text
chab tokens create [flags]
```

## Description

Create a bounded child API token after management approval.

The plaintext access token is returned once by the API and must be written to
--secret-out. The CLI reserves that private path before the HTTP request and
prints only a safe receipt. Live create also requires --approval-proof-file or
--approval-proof-prompt; the proof is sent only in the X-Chab-Management-Approval
header.

Build the request from flags or provide the exact JSON object with --body or
--body-file. Flag mode requires --name and at least one --permission. Optional
authority fields include --feature-mode, --feature-scope, --spending-mode,
--allowance-credits, --allowance-credits-null, --start-new-allowance,
--project-mode, --project-id, --ip-rule and --expires-at.

Use --dry-run to preview the request without reserving --secret-out, resolving
credentials, reading proof input, or contacting the API. Mutating requests send
an idempotency key; pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default safe receipt, --plain, --json, --jq, and --template.

Related commands:
- [chab tokens approvals create](chab-tokens-approvals-create.md)
- [chab tokens approvals wait](chab-tokens-approvals-wait.md)

## Examples

```text
  chab tokens create --name worker --permission api:projects:read --spending-mode none --project-mode none --approval-proof-file proof.json --secret-out token.json
  chab tokens create --body-file token.json --approval-proof-file proof.json --secret-out secret.json --json
  chab tokens create --name worker --permission api:projects:read --dry-run
  chab tokens create --name worker --permission api:projects:read --approval-proof-file proof.json --secret-out token.json --jq .secret_out
  chab tokens create --name worker --permission api:projects:read --approval-proof-file proof.json --secret-out token.json --template '{{.secret_out}}'
```

## Flags

- `--allowance-credits` - spending allowance credits
- `--allowance-credits-null` - send allowance_credits as null
- `--approval-proof-file` - private approval proof JSON file
- `--approval-proof-prompt` - read approval proof from a hidden prompt
- `--body` - JSON request body
- `--body-file` - read the JSON request body from a file, or - for stdin
- `--dry-run` - preview the request without resolving credentials or contacting the API
- `--expires-at` - expiration date-time
- `--expires-at-null` - send expires_at as null
- `--feature-mode` - feature access mode: none, selected, or all
- `--feature-scope` - feature scope (repeatable)
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--ip-rule` - IP rule allow=<address-or-cidr> or deny=<address-or-cidr> (repeatable)
- `--name` - token name
- `--permission` - token permission scope (repeatable)
- `--project-id` - numeric project id grant (repeatable)
- `--project-mode` - project access mode: none, selected, or all
- `--secret-out` - private path for the one-time plaintext token
- `--spending-mode` - spending mode: none, capped, or all
- `--start-new-allowance` - start a new allowance version

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, metadata
