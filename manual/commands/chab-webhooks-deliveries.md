# chab webhooks deliveries

## Usage

```text
chab webhooks deliveries [flags]
```

## Description

Manage webhook deliveries and individual replay requests.

Delivery replay checks current endpoint ownership, subscription, scope,
project and connected-data authority on the server. Replay commands require
local confirmation.

Related commands:
- [chab webhooks endpoints list](chab-webhooks-endpoints-list.md)
- [chab webhooks replays create](chab-webhooks-replays-create.md)

## Subcommands

- [chab webhooks deliveries list](chab-webhooks-deliveries-list.md) - List webhook deliveries
- [chab webhooks deliveries replay](chab-webhooks-deliveries-replay.md) - Replay a webhook delivery
- [chab webhooks deliveries show](chab-webhooks-deliveries-show.md) - Show a webhook delivery

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
