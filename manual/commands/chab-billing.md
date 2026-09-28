# chab billing

## Usage

```text
chab billing [flags]
```

## Description

Manage billing settings and purchases.

Billing write operations require manually created team API tokens with the
server-required billing scopes when browser device login cannot obtain them.
The server owns plan gates, purchase policy, auto-recharge authority and
management approval validation.

Related commands:
- [chab credits balance](chab-credits-balance.md)
- [chab tokens approvals create](chab-tokens-approvals-create.md)

## Subcommands

- [chab billing auto-recharge](chab-billing-auto-recharge.md) - Manage auto-recharge
- [chab billing packages](chab-billing-packages.md) - List billing packages
- [chab billing purchases](chab-billing-purchases.md) - Manage billing purchases
- [chab billing reconciliation](chab-billing-reconciliation.md) - Show billing reconciliation
- [chab billing show](chab-billing-show.md) - Show billing context

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
