# chab health

## Usage

```text
chab health [flags]
```

## Description

Show public API health.

This command calls unauthenticated GET /v1/health and never reads credentials.

Related commands:
- [chab errors](chab-errors.md)
- [chab doctor](chab-doctor.md)

```text
chab api get /health
```

## Examples

```text
  chab health
  chab health --plain
  chab health --json
  chab health --jq .status
  chab health --template '{{.status}}'
```

## Output modes

human, JSON, plain, jq, template

## Authentication

Runs without requiring a credential.
