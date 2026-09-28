# chab project create

## Usage

```text
chab project create [flags]
```

## Description

Create a project for the active team.

Build the request body from field flags or from a JSON body. Set individual
fields with --name (required), --description, --description-null, --url,
--url-null, --status (active, paused, archived), --timezone, --language,
--limit, and --automate; only the flags you set are sent, so --description ""
sends an explicit empty value while --description-null sends JSON null. Field
flags cannot be combined with body input. Alternatively pass the whole body as
JSON with --body '<json>' or --body-file <path>, and use --body-file - to read
the body from stdin. JSON body input must be a top-level object whose keys are a
subset of the documented fields with the documented scalar types; description
and url may be null, and field-value rules such as a required name are checked
by the API.

--limit is the project resource's integer limit field sent in the body; it is
not a pagination control here. --language is the project content language sent
in the body and is independent of the global --locale flag, which only sets the
request Accept-Language header.

Every real create sends an idempotency key so a retried request is not applied
twice. A key is generated automatically; pass --idempotency-key <key> to supply
your own (1-255 visible ASCII bytes). Key values are sensitive: output reports
only whether a key is generated or explicit and never prints the value. Use
--dry-run to preview the method, path, body fields, and idempotency behavior
without resolving credentials or contacting the API.

JSON output is the bare project object with fields id, name, description, url,
status, timezone, language, limit, automate, created_at, updated_at. Default
JSON without --include-meta has no request, replay, idempotency, or metadata
wrapper.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged. It cannot be combined with --dry-run.

Output modes: default human summary, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
- [chab project list](chab-project-list.md)
- [chab project show](chab-project-show.md)

## Examples

```text
  chab project create --name "Demo"
  chab project create --name "Demo" --status active --description ""
  chab project create --name "Demo" --description-null --url-null
  chab project create --body '{"name":"Demo","status":"active"}'
  chab project create --body-file project.json
  chab project create --name "Demo" --dry-run
  chab project create --name "Demo" --idempotency-key <key>
  chab project create --name "Demo" --json
  chab project create --name "Demo" --json --include-meta
  chab project create --name "Demo" --jq .id
  chab project create --name "Demo" --template '{{.id}}'
```

## Flags

- `--automate` - project automate flag (sent in the body)
- `--body` - JSON request body (mutually exclusive with field flags and --body-file)
- `--body-file` - read the JSON request body from a file, or - for stdin
- `--description` - project description (sent in the body; --description "" clears it)
- `--description-null` - send description as null
- `--dry-run` - preview the request without resolving credentials or contacting the API
- `--idempotency-key` - explicit idempotency key (1-255 visible ASCII bytes); generated when omitted
- `--language` - project content language (sent in the body; distinct from the global --locale)
- `--limit` - project resource limit field (sent in the body; not a pagination control)
- `--name` - project name (sent in the body)
- `--status` - project status: active, paused, or archived
- `--timezone` - project timezone (sent in the body; validated by the API)
- `--url` - project URL (sent in the body)
- `--url-null` - send url as null

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

dry-run, idempotency, metadata
