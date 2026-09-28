# chab errors

## Usage

```text
chab errors [flags]
```

## Description

List public API error codes.

This command calls unauthenticated GET /v1/errors and never reads credentials.

Related commands:
- [chab health](chab-health.md)
- [chab doctor](chab-doctor.md)

```text
chab api get /errors
```

## Examples

```text
  chab errors
  chab errors --plain
  chab errors --json
  chab errors --jq '.errors[] | select(.retryable)'
  chab errors --template '{{range .errors}}{{.code}}{{"\n"}}{{end}}'
```

## Output modes

human, JSON, plain, jq, template

## Authentication

Runs without requiring a credential.
