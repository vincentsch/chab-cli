# chab billing auto-recharge

## Usage

```text
chab billing auto-recharge [flags]
```

## Description

Manage billing auto-recharge.

Auto-recharge updates require an exact management approval proof. Create one
with chab tokens approvals create, complete verification in the product web
app, wait for a private proof file, then submit the update with
--approval-proof-file. --yes confirms only the update; it does not supply a
proof, package, budget, or missing request field.

Related commands:
- [chab tokens approvals create](chab-tokens-approvals-create.md)
- [chab billing purchases create](chab-billing-purchases-create.md)

## Subcommands

- [chab billing auto-recharge show](chab-billing-auto-recharge-show.md) - Show auto-recharge
- [chab billing auto-recharge update](chab-billing-auto-recharge-update.md) - Update auto-recharge

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
