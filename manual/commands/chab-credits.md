# chab credits

## Usage

```text
chab credits [flags]
```

## Description

Inspect team credits through the product API.

Balance inspection and cursor-paginated transaction history are available as
team-level reads.

Credits are a team-level resource rather than Project-scoped. A key with
credits read capability reports the whole team's balance and transactions.

Related commands:
- [chab login](chab-login.md)
- [chab health](chab-health.md)

## Subcommands

- [chab credits balance](chab-credits-balance.md) - Show the available credit balance
- [chab credits transactions](chab-credits-transactions.md) - List credit transactions

## Examples

```text
  chab credits balance
  chab credits transactions
```

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
