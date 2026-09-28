# chab operations estimate

## Usage

```text
chab operations estimate <operation-key> [flags]
```

## Description

Estimate an operation request.

Estimates are advisory. They validate and price the supplied input where the
backend supports generic estimates, but they are not atomic spending
guarantees for a later live request.
JSON output is the estimate data returned by the API. Use --plain for compact
script output.

Related commands:
- [chab operations start](chab-operations-start.md)
- [chab operations schema](chab-operations-schema.md)

## Examples

```text
  chab operations estimate search.web --input @request.json
  chab operations estimate search.web --input @request.json --json
  chab operations estimate search.web --input @request.json --plain
```

## Flags

- `--input` - JSON request body, @path, or @- for stdin
- `--set` - top-level request field as name=json (repeatable)

## Output modes

human, JSON, plain

## Authentication

Requires a stored credential or `CHAB_API_KEY`.
