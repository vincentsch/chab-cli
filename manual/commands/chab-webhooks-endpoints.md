# chab webhooks endpoints

## Usage

```text
chab webhooks endpoints [flags]
```

## Description

Manage persistent webhook endpoints.

Endpoint list and show never include signing secrets. Create and rotate-secret
write one-time secrets only to --secret-out.

Related commands:
- [chab webhooks deliveries list](chab-webhooks-deliveries-list.md)
- [chab webhooks replays create](chab-webhooks-replays-create.md)

## Subcommands

- [chab webhooks endpoints create](chab-webhooks-endpoints-create.md) - Create a webhook endpoint
- [chab webhooks endpoints delete](chab-webhooks-endpoints-delete.md) - Delete a webhook endpoint
- [chab webhooks endpoints list](chab-webhooks-endpoints-list.md) - List webhook endpoints
- [chab webhooks endpoints rotate-secret](chab-webhooks-endpoints-rotate-secret.md) - Rotate a webhook secret
- [chab webhooks endpoints show](chab-webhooks-endpoints-show.md) - Show a webhook endpoint
- [chab webhooks endpoints update](chab-webhooks-endpoints-update.md) - Update a webhook endpoint

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
