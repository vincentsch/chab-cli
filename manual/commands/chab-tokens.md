# chab tokens

## Usage

```text
chab tokens [flags]
```

## Description

Manage team API tokens.

Token write operations use server-owned policy revisions and management
approval where required. Browser device login currently grants only
read-oriented management scopes; use a manually created team API key when the
server rejects token write scopes.

One-time token secrets and approval proofs are never printed. Use --secret-out
or --proof-out to save them to private files.

Related commands:
- [chab whoami](chab-whoami.md)
- [chab tokens approvals create](chab-tokens-approvals-create.md)

## Subcommands

- [chab tokens approvals](chab-tokens-approvals.md) - Manage approval challenges
- [chab tokens create](chab-tokens-create.md) - Create an API token
- [chab tokens list](chab-tokens-list.md) - List API tokens
- [chab tokens revoke](chab-tokens-revoke.md) - Revoke an API token
- [chab tokens show](chab-tokens-show.md) - Show API token metadata
- [chab tokens update](chab-tokens-update.md) - Update an API token

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
