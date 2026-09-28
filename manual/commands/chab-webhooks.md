# chab webhooks

## Usage

```text
chab webhooks [flags]
```

## Description

Manage persistent webhook endpoints, deliveries and replay requests.

Webhook endpoint creation and secret rotation return one-time signing secrets.
Those commands require --secret-out and save the secret only to that private
file. Replay and delete commands require confirmation; --yes confirms only the
prompt and does not supply missing request fields.

Browser device login may not grant webhook scopes. Use a manually created team
API key when the server rejects broader scopes.

Related commands:
- [chab operations wait](chab-operations-wait.md)
- [chab credits balance](chab-credits-balance.md)

## Subcommands

- [chab webhooks deliveries](chab-webhooks-deliveries.md) - Manage webhook deliveries
- [chab webhooks endpoints](chab-webhooks-endpoints.md) - Manage webhook endpoints
- [chab webhooks replays](chab-webhooks-replays.md) - Manage webhook replay windows

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
