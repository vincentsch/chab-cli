# chab tokens approvals

## Usage

```text
chab tokens approvals [flags]
```

## Description

Manage human approval challenges for protected token and billing actions.

The CLI can create a challenge and poll for a one-time proof after a human
approves it in the product web app. It cannot approve its own request. Proof
polling uses one physical attempt per API call; ambiguous proof responses are
not retried automatically.

Related commands:
- [chab tokens revoke](chab-tokens-revoke.md)
- [chab billing purchases create](chab-billing-purchases-create.md)

## Subcommands

- [chab tokens approvals create](chab-tokens-approvals-create.md) - Create an approval challenge
- [chab tokens approvals wait](chab-tokens-approvals-wait.md) - Wait for an approval proof

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
