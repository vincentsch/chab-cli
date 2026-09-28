# chab tokens update

## Usage

```text
chab tokens update <token-public-id> [flags]
```

## Description

Update API token policy or lifecycle metadata.

Flag mode requires --revision and at least one change. Use --approval-proof-file
or --approval-proof-prompt for cross-token or authority-widening mutations that
the server protects with management approval. The proof is sent only in
X-Chab-Management-Approval.

Authority-related changes require confirmation; --yes confirms only the risky
mutation and does not supply a proof, revision, budget, project grant or body
field. In --no-prompt or non-interactive mode, pass --yes after providing all
required inputs. Use --dry-run to preview the request without credentials,
proof input, confirmation or HTTP. Mutating requests send an idempotency key;
pass --idempotency-key to provide your own.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. It cannot be combined with --dry-run.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab tokens show](chab-tokens-show.md)
- [chab tokens approvals create](chab-tokens-approvals-create.md)

## Examples

```text
  chab tokens update <token-public-id> --revision 3 --disabled=true
  chab tokens update <token-public-id> --revision 3 --spending-mode capped --allowance-credits 100 --approval-proof-file proof.json --yes
  chab tokens update <token-public-id> --body-file mutation.json --approval-proof-file proof.json --yes --json
  chab tokens update <token-public-id> --revision 3 --name worker --jq .token.name
  chab tokens update <token-public-id> --revision 3 --name worker --template '{{.token.name}}'
```

## Flags

- `--allowance-credits` - spending allowance credits
- `--allowance-credits-null` - send allowance_credits as null
- `--approval-proof-file` - private approval proof JSON file
- `--approval-proof-prompt` - read approval proof from a hidden prompt
- `--body` - JSON request body
- `--body-file` - read the JSON request body from a file, or - for stdin
- `--disabled` - enable or disable the token
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
- `--revision` - expected token policy revision
- `--spending-mode` - spending mode: none, capped, or all
- `--start-new-allowance` - start a new allowance version

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, metadata, confirmation
