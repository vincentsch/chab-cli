# chab translate text-or-document

## Usage

```text
chab translate text-or-document [flags]
```

## Description

Start a text or document translation.

This command resolves convenience flags and --input into one request body,
creates a private action record before live submission, and sends exactly one
idempotency key for the logical action. Use --dry-run for a local preview; to
request a server dry run, include "dry_run": true in the JSON body when the
operation supports it.

Paid live submissions require confirmation through --yes or an interactive
prompt. JSON output is one stable object with action, remote operation and
recovery fields. Use --plain to print the accepted operation id or action id.
In --no-prompt or non-interactive mode, use --yes to acknowledge paid live
work. When the operation schema supports dry_run, an input body with
"dry_run": true performs authenticated server validation without creating a
local action record or sending an idempotency key. Pass --idempotency-key to
supply a caller-owned replay key for live work.

Related commands:

```text
chab operations schema translate.text_or_document
```

- [chab operations resume](chab-operations-resume.md)

## Examples

```text
  chab translate text-or-document --source '{"type":"inline","format":"text","content":"Hallo"}' --target-language en --max-credits 25 --yes
```

## Flags

- `--dry-run` - preview the resolved request without resolving credentials or contacting the API
- `--formality` - translation formality
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--input` - JSON request body, @path, or @- for stdin
- `--max-credits` - set max_credit_budget for supported operations
- `--set` - top-level request field as name=json (repeatable)
- `--source` - source object as JSON
- `--source-language` - source language
- `--style` - translation style
- `--target-language` - target language
- `--wait` - wait for an accepted operation to reach a terminal state

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, confirmation
