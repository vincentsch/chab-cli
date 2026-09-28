# chab mail drafts

## Usage

```text
chab mail drafts [flags]
```

## Description

Work with API-owned mail drafts.

Draft mutations are synchronous Chab API calls. Live mutations use one
Idempotency-Key per intended change, and --dry-run asks the server to validate
without creating or changing provider data. Draft attachments accept eligible
operation artifact IDs only.

Related commands:
- [chab mail personas](chab-mail-personas.md)
- [chab operations artifact](chab-operations-artifact.md)

## Subcommands

- [chab mail drafts attachments](chab-mail-drafts-attachments.md) - Work with draft attachments
- [chab mail drafts create](chab-mail-drafts-create.md) - Create a mail draft (preview)
- [chab mail drafts discard](chab-mail-drafts-discard.md) - Discard a mail draft (preview)
- [chab mail drafts send](chab-mail-drafts-send.md) - Send a mail draft (preview)
- [chab mail drafts show](chab-mail-drafts-show.md) - Show a mail draft (preview)
- [chab mail drafts update](chab-mail-drafts-update.md) - Update a mail draft (preview)

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
