# chab operations actions

## Usage

```text
chab operations actions [flags]
```

## Description

Inspect local action records.

Action output is a public projection. It omits stored idempotency keys, request
hashes and other private recovery material.
Use --json or --plain on the list and show leaves.

Related commands:
- [chab operations resume](chab-operations-resume.md)

## Subcommands

- [chab operations actions list](chab-operations-actions-list.md) - List local action records
- [chab operations actions show](chab-operations-actions-show.md) - Show a local action record

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
