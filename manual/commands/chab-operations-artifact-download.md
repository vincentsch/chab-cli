# chab operations artifact download

## Usage

```text
chab operations artifact download <operation-id> <artifact-id> [flags]
```

## Description

Download operation artifact bytes to an explicit new local path.

The command first reads trusted artifact metadata, resolves the documented
download_url below the configured API base, and then downloads bytes from the
canonical artifact download endpoint. Existing output files are refused. Safe
external HTTPS redirects are followed with Chab authorization and cookies
stripped from the redirected request. When a sha256:<hex> checksum is available
or supplied through --checksum, the file is published only after the completed
transfer matches it.

--include-meta adds safe response metadata under meta when --json, --jq, or
--template is selected.

Output modes: default human detail, --plain, --json, --jq, and --template.

Related commands:
- [chab operations artifact](chab-operations-artifact.md)
- [chab operations result](chab-operations-result.md)

## Examples

```text
  chab operations artifact download op_123 art_123 --output ./result.md
  chab operations artifact download op_123 art_123 -o ./result.md --checksum sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --json
  chab operations artifact download op_123 art_123 -o ./result.md --jq .bytes
  chab operations artifact download op_123 art_123 -o ./result.md --template '{{.path}}'
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

metadata
