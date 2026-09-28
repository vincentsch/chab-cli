# chab project delete

## Usage

```text
chab project delete <project-id> [flags]
```

## Description

Delete a project visible to the active team API key.

Identify the project by its opaque id (sent to the API as-is, only checked
non-empty locally), so malformed, missing, deleted, cross-team, and out-of-scope
ids all return the API's not_found response. A successful response is
the deleted id and a Boolean deleted flag.

Deleting a project is destructive. In an interactive terminal the command asks
for confirmation before sending the request; the prompt uses a quoted, redacted
copy of the id and makes no claim about whether the project exists. Pass --yes
to confirm without a prompt. In non-interactive or --no-prompt mode a real
delete without --yes fails locally before any credential lookup or request.

Every real delete sends an idempotency key so a retried request is not applied
twice. A key is generated automatically; pass --idempotency-key <key> to supply
your own (1-255 visible ASCII bytes). Key values are sensitive: output reports
only whether a key is generated or explicit and never prints the value. Use
--dry-run to preview the method, path, and idempotency behavior without
confirming, resolving credentials, or contacting the API.

JSON output is exactly one object with string id and Boolean deleted fields.
Without --include-meta, transport and replay context is not shown.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged. It cannot be combined with --dry-run.

Output modes: default human summary, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
- [chab project list](chab-project-list.md)
- [chab credits](chab-credits.md)

## Examples

```text
  chab project delete <project-id>
  chab project delete <project-id> --yes
  chab project delete <project-id> --dry-run
  chab project delete <project-id> --idempotency-key <key>
  chab project delete <project-id> --json
  chab project delete <project-id> --json --include-meta
  chab project delete <project-id> --jq .deleted
  chab project delete <project-id> --template '{{.id}}'
```

## Flags

- `--dry-run` - preview the request without resolving credentials or contacting the API
- `--idempotency-key` - explicit idempotency key (1-255 visible ASCII bytes); generated when omitted

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, metadata, confirmation
