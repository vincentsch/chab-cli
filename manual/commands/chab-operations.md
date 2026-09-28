# chab operations

## Usage

```text
chab operations [flags]
```

## Description

Inspect and recover Chab operations.

Operation starts create a private local action record before sending the first
effectful request. The action record stores recovery metadata and an
idempotency key, but never stores a bearer token or request body.
Journal directories are created private where the platform supports file
permissions. Record writes sync the data file before atomic publication;
directory metadata sync is best effort for portability.

Schemas are read from the pinned Chab API contract embedded in the binary, so
schema inspection works offline.

Related commands:
- [chab search web](chab-search-web.md)
- [chab credits balance](chab-credits-balance.md)

## Subcommands

- [chab operations actions](chab-operations-actions.md) - Inspect local action records
- [chab operations artifact](chab-operations-artifact.md) - Fetch operation artifact metadata
- [chab operations bulk-cancel](chab-operations-bulk-cancel.md) - Cancel operations by filter
- [chab operations cancel](chab-operations-cancel.md) - Cancel an operation
- [chab operations estimate](chab-operations-estimate.md) - Estimate an operation request
- [chab operations list](chab-operations-list.md) - List remote operations
- [chab operations result](chab-operations-result.md) - Fetch operation result metadata
- [chab operations resume](chab-operations-resume.md) - Resume a local action
- [chab operations schema](chab-operations-schema.md) - Show an embedded operation schema
- [chab operations show](chab-operations-show.md) - Show operation status
- [chab operations start](chab-operations-start.md) - Start a supported operation by key
- [chab operations wait](chab-operations-wait.md) - Wait for an operation to finish

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
