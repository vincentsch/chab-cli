# chab operations start

## Usage

```text
chab operations start <operation-key> [flags]
```

## Description

Start a supported operation by operation key using a JSON request body.

The start command is the generic entrypoint for supported operation starts. It
creates or reuses one local action record and one idempotency key for the
logical action before sending the effectful request.
Paid live submissions require confirmation through --yes or an interactive
prompt. Use --dry-run for a local preview that does not resolve credentials or
contact the API. For operations whose schema supports dry_run, an input body
with "dry_run": true performs authenticated server validation without creating
a local action record or sending an idempotency key. JSON output is one stable
object with action, remote operation and recovery fields. Use --plain to print
the accepted operation id or action id. In --no-prompt or non-interactive mode,
use --yes to acknowledge paid live work. Pass --idempotency-key to supply a
caller-owned replay key.

Related commands:
- [chab operations resume](chab-operations-resume.md)
- [chab operations wait](chab-operations-wait.md)
- [chab operations schema](chab-operations-schema.md)

## Examples

```text
  chab operations start search.web --input @request.json --yes
  chab operations start search.web --input @request.json --yes --wait --json
  chab operations start search.web --input @request.json --dry-run
  chab operations start examples.echo --input @request.json --plain
```

## Flags

- `--dry-run` - preview the resolved request without resolving credentials or contacting the API
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--input` - JSON request body, @path, or @- for stdin
- `--max-credits` - set max_credit_budget for supported operations
- `--set` - top-level request field as name=json (repeatable)
- `--wait` - wait for an accepted operation to reach a terminal state

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, confirmation
