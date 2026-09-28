# chab billing purchases

## Usage

```text
chab billing purchases [flags]
```

## Description

Manage credit package purchases.

Purchase creation is a resource lifecycle. The create command records a local
idempotent action before submission, follows the returned purchase resource,
and never creates a replacement purchase to resolve an ambiguous outcome.
Purchase mutations require exact human approval and local confirmation.

Related commands:
- [chab billing packages](chab-billing-packages.md)
- [chab tokens approvals create](chab-tokens-approvals-create.md)

## Subcommands

- [chab billing purchases create](chab-billing-purchases-create.md) - Create a credit purchase
- [chab billing purchases show](chab-billing-purchases-show.md) - Show a credit purchase
- [chab billing purchases wait](chab-billing-purchases-wait.md) - Wait for a credit purchase

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
