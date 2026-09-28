# chab project archive

## Usage

```text
chab project archive <project-id> [flags]
```

## Description

Archive a project visible to the active team API key.

Identify the project by its opaque id. Every non-empty value is sent as one
escaped API path segment without local normalization. The command sends exactly
one PATCH body: {"status":"archived"}. The API remains authoritative for
transition validation: the CLI does not read the project first or enforce a
transition graph.

Every real update sends an idempotency key so a retried request is not applied
twice. A key is generated automatically; pass --idempotency-key <key> to supply
your own (1-255 visible ASCII bytes). Key values are sensitive: output reports
only whether a key is generated or explicit and never prints the value. Use
--dry-run to preview the method, path, body fields, and idempotency behavior
without resolving credentials or contacting the API.

JSON output is the bare project object with fields id, name, description, url,
status, timezone, language, limit, automate, created_at, updated_at. Every field
comes from the API response, including status; the requested status is never
substituted into output. Default JSON without --include-meta has no request,
replay, idempotency, or metadata wrapper.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged. It cannot be combined with --dry-run.

Output modes: default human summary, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

This reversible status update does not ask for confirmation; inherited --yes
does not change execution. Use chab project update when changing status together
with other project fields or when supplying a generic JSON change set.

Related commands:
- [chab project show](chab-project-show.md)
- [chab project update](chab-project-update.md)

## Examples

```text
  chab project archive <project-id>
  chab project archive <project-id> --dry-run --json
  chab project archive <project-id> --jq .status
  chab project archive <project-id> --include-meta --template '{{.meta.request_id}}'
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

dry-run, idempotency, metadata
