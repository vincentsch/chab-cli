# chab mail messages

## Usage

```text
chab mail messages [flags]
```

## Description

Work with mail messages.

Message commands read sanitized bodies or apply the documented message-state
actions. State updates are synchronous Chab API calls with revision checks and
server dry-run support.

Related commands:
- [chab mail threads](chab-mail-threads.md)
- [chab mail attachments download](chab-mail-attachments-download.md)

## Subcommands

- [chab mail messages body](chab-mail-messages-body.md) - Read a mail message body (preview)
- [chab mail messages state](chab-mail-messages-state.md) - Update mail message state (preview)

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
