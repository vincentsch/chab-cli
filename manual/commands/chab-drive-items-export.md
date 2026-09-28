# chab drive items export

## Usage

```text
chab drive items export <item-id> [flags]
```

## Description

Export a drive-native document to a Chab operation artifact.

Supply project_id, connection_id and output_format with flags, --input, or
--set. Live requests use an Idempotency-Key and the accepted-operation
lifecycle; use --idempotency-key to supply one or let the CLI generate it. Use
--wait to wait for the accepted operation to finish.

Output modes: default human detail, --plain, --json.

Related commands:
- [chab operations wait](chab-operations-wait.md)
- [chab operations artifact download](chab-operations-artifact-download.md)

Preview: this command uses a preview Chab API contract.

## Examples

```text
  chab drive items export dri_123 --project-id 42 --connection-id 7 --output-format pdf
  chab drive items export dri_123 --input @drive-export.json --wait --json
```

## Flags

- `--connection-id` - numeric drive connection id
- `--idempotency-key` - explicit idempotency key; generated when omitted
- `--input` - JSON request body, @path, or @- for stdin
- `--output-format` - export format: pdf, text, html, or markdown
- `--project-id` - numeric Chab project id
- `--set` - top-level request field as name=json (repeatable)
- `--wait` - wait for an accepted operation to reach a terminal state

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.

## Help topics

idempotency, preview
