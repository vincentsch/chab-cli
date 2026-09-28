# chab files show

## Usage

```text
chab files show <file-id> [flags]
```

## Description

Show stored file metadata.

The stored-file show endpoint rejects generic project or provider query
context; visibility is resolved by the active API key.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab files download](chab-files-download.md)
- [chab files wait](chab-files-wait.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab files show fil_123
  chab files show fil_123 --json --include-meta
  chab files show fil_123 --jq .file.id
  chab files show fil_123 --template '{{.file.id}}'
```

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata, preview
