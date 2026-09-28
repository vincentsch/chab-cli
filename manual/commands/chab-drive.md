# chab drive

## Usage

```text
chab drive [flags]
```

## Description

Work with connected drive files.

Drive commands use Chab project and connection scoping. Read commands return
provider-neutral metadata. Downloads require an explicit local output path.
Export and write commands use the accepted-operation lifecycle and local action
records. Request the required drive scopes during browser device login, and
approve project access and spending separately. Connected-drive eligibility
and the current connection permissions still apply.

Related commands:
- [chab files upload](chab-files-upload.md)
- [chab operations wait](chab-operations-wait.md)
- [chab operations artifact download](chab-operations-artifact-download.md)

## Subcommands

- [chab drive connections](chab-drive-connections.md) - List drive connections (preview)
- [chab drive folders](chab-drive-folders.md) - Work with drive folders
- [chab drive items](chab-drive-items.md) - Work with drive items
- [chab drive permissions](chab-drive-permissions.md) - Work with drive permissions
- [chab drive search](chab-drive-search.md) - Search drive item names (preview)

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
