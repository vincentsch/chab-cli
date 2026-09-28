# chab api patch

## Usage

```text
chab api patch <path> [flags]
```

## Description

Send a PATCH request to an API path using the active profile and credential.

<path> is API-relative and is joined below the resolved API base. Give a path
like /projects/<id> or projects/<id>. A bare first segment containing a colon is
rejected as ambiguous URI syntax; a colon after an explicit leading slash or in
a later segment remains opaque path data. Absolute and scheme-relative URLs,
query or fragment markers, trailing slashes, and empty segments fail locally.
Put query parameters in --query key=value (repeatable); values may be empty or
contain commas and equals signs, and repeated values retain their order.

Build a JSON body with --field key=value (repeatable), pass any valid JSON value
with --body '<json>', or read one with --body-file <path>. Use --body-file - to
read from stdin. These body modes are mutually exclusive, and duplicate --field
keys fail locally. A PATCH with no body sends no Content-Type and no JSON null
body.

Every real PATCH sends an idempotency key so a retried request is not applied
twice. A key is generated automatically; pass --idempotency-key <key> to supply
your own (1-255 visible ASCII bytes). Key values are sensitive: output reports
only whether a key is generated or explicit and never prints the value. Use
--dry-run to preview the method, path, query, body, and idempotency behavior
without resolving credentials or contacting the API.
Catalogued idempotent routes create a private recovery record after resolving
credential identity and before the effectful request.
Those catalogued recoverable routes print the stable action and recovery object,
with the API success payload under server. Other raw PATCH routes keep printing
the selected API success value.
Catalogued operation routes that support JSON body "dry_run": true send
authenticated server validation without confirmation, a recovery record, or an
idempotency key.

PATCH is an unsafe raw action and does not ask for confirmation. Choosing this
leaf is the explicit decision to send it; --yes is not required.

For non-catalogued raw routes, the decoded "data" value of the API success
envelope is printed by default. Use --raw to print the full success envelope,
including data, meta, request_id, and unknown envelope keys. --raw and
--dry-run cannot be combined.

Mark a request value as sensitive with --secret-field <name> (repeatable). Named
query values, field values, and matching JSON body keys are shown as [REDACTED]
in dry-run previews, --debug output, and rendered errors; the flag does not add
a request value.

JSON output is the selected machine value as stable JSON. Default human and
--plain output print scalar values without JSON quotes; objects, arrays, and
null print as JSON because the raw shape is not known ahead of time.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged. It cannot be combined with --dry-run.

Output modes: default human, --plain, --json stable JSON, and --jq/--template
transforms over the selected JSON value.

Related commands:
- [chab api](chab-api.md)
- [chab login](chab-login.md)

## Examples

```text
  chab api patch /projects/<id> --field status=active
  chab api patch /projects/<id> --body '{"status":"paused"}' --json
  chab api patch /projects/<id> --body-file payload.json --dry-run
  chab api patch /projects/<id> --field status=active --idempotency-key <key>
  chab api patch /projects/<id> --jq .status
  chab api patch /projects/<id> --field status=active --template '{{.status}}'
  chab api patch /projects/<id> --field status=active --include-meta --template '{{.meta.request_id}}'
  chab api patch /projects/<id> --body '{"token":"<token>"}' --secret-field token --debug
```

## Flags

- `--body` - JSON request body (mutually exclusive with --field and --body-file)
- `--body-file` - read the JSON request body from a file, or - for stdin
- `--dry-run` - preview the request without resolving credentials or contacting the API
- `--field` - body field as key=value (repeatable; mutually exclusive with --body and --body-file)
- `--idempotency-key` - explicit idempotency key (1-255 visible ASCII bytes); generated when omitted on replayable unsafe routes
- `--query` - query parameter as key=value (repeatable)
- `--raw` - print the full API success envelope instead of only data
- `--secret-field` - request field name whose value must be redacted in debug output and errors (repeatable)

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

raw-envelope, dry-run, idempotency, metadata
