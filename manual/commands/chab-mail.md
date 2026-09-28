# chab mail

## Usage

```text
chab mail [flags]
```

## Description

Work with connected mail.

Chab Mail commands expose the documented synchronous Mail API only. They do
not add inherited send-status, scheduling, browser-draft adoption, management
approval, arbitrary local attachment upload, or provider-native controls.
Request the required mail scopes during browser device login. The team must
also have mail access, an eligible connection and the required project grants.

Related commands:
- [chab setup](chab-setup.md)
- [chab files](chab-files.md)
- [chab operations actions](chab-operations-actions.md)

## Subcommands

- [chab mail attachments](chab-mail-attachments.md) - Work with message attachments
- [chab mail connections](chab-mail-connections.md) - List mail connections (preview)
- [chab mail drafts](chab-mail-drafts.md) - Work with API-owned mail drafts
- [chab mail folders](chab-mail-folders.md) - List mail folders (preview)
- [chab mail messages](chab-mail-messages.md) - Work with mail messages
- [chab mail personas](chab-mail-personas.md) - List mail personas (preview)
- [chab mail search](chab-mail-search.md) - Search connected mail (preview)
- [chab mail threads](chab-mail-threads.md) - Work with mail threads

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
