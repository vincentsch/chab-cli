# chab mail attachments download

## Usage

```text
chab mail attachments download <message-id> <attachment-id> [flags]
```

## Description

Download one mail attachment to an explicit new local path.

Mail attachment downloads are direct streams with no redirect support. Existing
output files are refused. When --checksum supplies a sha256:<hex> value, the
file is published only after the completed transfer matches it.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default human detail, --plain, --json, --jq, and --template.

Related commands:
- [chab mail threads show](chab-mail-threads-show.md)
- [chab files download](chab-files-download.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab mail attachments download 84 att_123 --project-id 42 --connection-id 7 --output ./attachment.pdf
  chab mail attachments download 84 att_123 --project-id 42 --connection-id 7 -o ./attachment.pdf --json
  chab mail attachments download 84 att_123 --project-id 42 --connection-id 7 -o ./attachment.pdf --jq .bytes
  chab mail attachments download 84 att_123 --project-id 42 --connection-id 7 -o ./attachment.pdf --template '{{.path}}'
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
