# chab project show

## Usage

```text
chab project show <project-id> [flags]
```

## Description

Show a project visible to the active team API key.

The project id is opaque: every non-empty value is sent to the API as one path
segment without trimming, decoding, normalization, or local shape validation.
Malformed, deleted, cross-team, and out-of-scope ids all return the API's
not_found response.

JSON output is one project object with fields id, name, description, url,
status, timezone, language, limit, automate, created_at, updated_at.

--include-meta adds safe transport context under meta when --json, --jq, or
--template is selected. The ordinary value moves under data; output without
--include-meta remains unchanged.

Output modes: default human detail, --plain tab-separated fields, --json stable
JSON, and --jq/--template transforms over the documented JSON value.

Related commands:
- [chab project list](chab-project-list.md)
- [chab credits](chab-credits.md)

## Examples

```text
  chab project show <project-id>
  chab project show <project-id> --json
  chab project show <project-id> --json --include-meta
  chab project show <project-id> --jq .status
  chab project show <project-id> --template '{{.id}} {{.name}}'
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata
