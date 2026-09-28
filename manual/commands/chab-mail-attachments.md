# chab mail attachments

## Usage

```text
chab mail attachments [flags]
```

## Description

Work with message attachments.

Message attachments are downloaded as untrusted direct byte streams. The CLI
never uses a server filename as the local path and never overwrites an existing
file.

Related commands:
- [chab mail messages body](chab-mail-messages-body.md)
- [chab files download](chab-files-download.md)

## Subcommands

- [chab mail attachments download](chab-mail-attachments-download.md) - Download a mail attachment (preview)

## Output modes

none (command family)

## Authentication

Runs without requiring a credential.
