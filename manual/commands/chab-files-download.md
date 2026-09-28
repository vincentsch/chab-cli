# chab files download

## Usage

```text
chab files download <file-id> [flags]
```

## Description

Download stored file bytes to an explicit new local path.

The command first reads trusted file metadata, resolves the documented
download_url below the configured API base, and streams bytes through the
stored-file download endpoint without adding project or provider query
context. Existing output files are refused. When a sha256:<hex> checksum is
available or supplied through --checksum, the file is published only after the
completed transfer matches it.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default human detail, --plain, --json, --jq, and --template.

Related commands:
- [chab files show](chab-files-show.md)
- [chab operations artifact download](chab-operations-artifact-download.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab files download fil_123 --output ./contract.pdf
  chab files download fil_123 -o ./contract.pdf --checksum sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --json
  chab files download fil_123 -o ./contract.pdf --jq .bytes
  chab files download fil_123 -o ./contract.pdf --template '{{.path}}'
```

## Flags

- `--checksum` - expected sha256:<hex> checksum
- `--max-bytes` - maximum bytes to write
- `--output` - new local output path

## Output modes

human, JSON, plain, jq, template

## Metadata

`--include-meta` is supported. With `--json`, `--jq`, or `--template`, the command value is wrapped under `data` and response, pagination, rate-limit, retry, and request-id metadata is exposed under `meta`.

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

metadata, preview
