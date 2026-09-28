# chab files wait

## Usage

```text
chab files wait <file-id> [flags]
```

## Description

Wait for a stored file to leave scanner-pending state.

The command polls the stored-file resource and honors Retry-After when the
server provides it. It treats available as success and returns a local failure
when the server reports scan_failed, quarantined, or expired.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default JSON-like detail, --plain, --json, --jq, and --template.

Related commands:
- [chab files upload](chab-files-upload.md)
- [chab files show](chab-files-show.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab files wait fil_123
  chab files wait fil_123 --timeout 2m --json
  chab files wait fil_123 --jq .file.state
  chab files wait fil_123 --template '{{.file.state}}'
```

## Flags

- `--timeout` - maximum local wait duration

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata, preview
