# chab drive items download

## Usage

```text
chab drive items download <item-id> [flags]
```

## Description

Download one ordinary drive file to an explicit new local path.

Drive downloads may be streamed by Chab or returned as a safe short-lived HTTPS
redirect. Redirect follow-up requests strip Chab authorization and cookies.
Existing output files are refused. When --checksum supplies a sha256:<hex>
value, the file is published only after the completed transfer matches it.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default human detail, --plain, --json, --jq, and --template.

Related commands:
- [chab drive items show](chab-drive-items-show.md)
- [chab operations artifact download](chab-operations-artifact-download.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab drive items download dri_123 --project-id 42 --connection-id 7 --output ./report.pdf
  chab drive items download dri_123 --project-id 42 --connection-id 7 -o ./report.pdf --json
  chab drive items download dri_123 --project-id 42 --connection-id 7 -o ./report.pdf --jq .bytes
  chab drive items download dri_123 --project-id 42 --connection-id 7 -o ./report.pdf --template '{{.path}}'
```

## Flags

- `--checksum` - expected sha256:<hex> checksum
- `--connection-id` - numeric connected provider connection id
- `--max-bytes` - maximum bytes to write
- `--output` - new local output path
- `--project-id` - numeric Chab project id

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata, preview
