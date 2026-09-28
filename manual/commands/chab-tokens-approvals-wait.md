# chab tokens approvals wait

## Usage

```text
chab tokens approvals wait <approval-id> [flags]
```

## Description

Wait for a management approval proof and write it to a private file.

The CLI polls the approval endpoint with a no-retry API client because the
proof is returned once. Pending responses follow Retry-After when present and
otherwise use the server's next_check_at or a short fallback. If the server says
proof_issued without returning a proof, the one-time value is no longer
recoverable; create a fresh approval instead of polling indefinitely.

The proof file is JSON and includes the approval id, action, target, revision
where applicable, issuing token context, normalized mutation and proof value.
The proof itself is never printed.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected.

Output modes: default safe receipt, --plain, --json, --jq, and --template.

Related commands:
- [chab tokens revoke](chab-tokens-revoke.md)
- [chab billing purchases create](chab-billing-purchases-create.md)

## Examples

```text
  chab tokens approvals wait <approval-id> --proof-out proof.json
  chab tokens approvals wait <approval-id> --proof-out proof.json --json
  chab tokens approvals wait <approval-id> --proof-out proof.json --jq .secret_out
  chab tokens approvals wait <approval-id> --proof-out proof.json --template '{{.secret_out}}'
```

## Flags

- `--proof-out` - private path for the one-time approval proof

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
