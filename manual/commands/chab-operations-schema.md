# chab operations schema

## Usage

```text
chab operations schema <operation-key> [flags]
```

## Description

Show the embedded request and result schema for an operation.

Schema inspection reads the pinned contract built into the binary. It does not
resolve a profile, read credentials, or contact the API.
Use --json to print the embedded schema object or --plain to print the
operation key.

Related commands:
- [chab operations estimate](chab-operations-estimate.md)
- [chab operations start](chab-operations-start.md)

## Examples

```text
  chab operations schema search.web
  chab operations schema search.web --json
  chab operations schema search.web --plain
```

## Output modes

human, JSON, plain

## Authentication

Runs without requiring a credential.
